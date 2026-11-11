package inventory

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"time"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/entity"
	"github.com/cloudreve/Cloudreve/v4/ent/file"
	"github.com/cloudreve/Cloudreve/v4/ent/metadata"
	"github.com/cloudreve/Cloudreve/v4/ent/share"
	"github.com/cloudreve/Cloudreve/v4/ent/user"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/boolset"
)

const LecTeamRetention = 30 * 24 * time.Hour
const RestoreURIKey = "sys:restore_uri"
const TrashCollectTimeKey = "sys:expected_collect_time"

func IsLecTeamMember(u *ent.User) bool { return u != nil && u.LecTeamOwnerID != nil }

func IsLecTeamAdmin(u *ent.User) bool {
	return IsLecTeamMember(u) && u.Edges.Group != nil && u.Edges.Group.Permissions.Enabled(int(types.GroupPermissionIsAdmin))
}

// StorageUser 只替换文件归属，登录身份和管理员权限仍使用原成员账号。
func StorageUser(u *ent.User) *ent.User {
	if u == nil || u.LecTeamOwnerID == nil || *u.LecTeamOwnerID == u.ID {
		return u
	}
	owner, err := u.Edges.LecTeamOwnerOrErr()
	if err != nil {
		// 未加载团队关系属于调用错误，必须中止，不能回退到个人空间。
		panic(fmt.Errorf("团队文件归属未加载: %w", err))
	}
	return owner
}

// EnsureLecTeamOwner 创建无登录入口的团队资产账号，组织间独立保存资产。
func EnsureLecTeamOwner(ctx context.Context, db *ent.Client, issuer, organization string) (*ent.User, error) {
	key := sha256.Sum256([]byte(issuer + "\x00" + organization))
	email := fmt.Sprintf("team-%x@sso.invalid", key[:16])
	lookup := func() (*ent.User, error) { return db.User.Query().Where(user.Email(email)).WithGroup().Only(ctx) }
	if u, err := lookup(); !ent.IsNotFound(err) {
		return u, err
	}
	tx, err := db.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	base, err := tx.Group.Get(ctx, 2)
	if err != nil {
		return nil, err
	}
	admin, err := tx.Group.Get(ctx, 1)
	if err != nil {
		return nil, err
	}
	permissions := append(boolset.BooleanSet{}, (*base.Permissions)...)
	boolset.Set(types.GroupPermissionIsAdmin, false, &permissions)
	settings := *base.Settings
	settings.TrashRetention = int(LecTeamRetention / time.Second)
	settings.RedirectedSource = true
	createGroup := tx.Group.Create().SetName("LEC 团队空间").SetMaxStorage(admin.MaxStorage).
		SetPermissions(&permissions).SetSettings(&settings)
	if base.StoragePolicyID > 0 {
		createGroup.SetStoragePolicyID(base.StoragePolicyID)
	}
	group, err := createGroup.Save(ctx)
	if err != nil {
		return nil, err
	}
	password, err := digestPassword(rand.Text())
	if err != nil {
		return nil, err
	}
	u, err := tx.User.Create().SetEmail(email).SetNick("LEC 团队").SetGroupID(group.ID).SetPassword(password).
		SetSettings(&types.UserSetting{Language: "zh-CN", VersionRetention: true}).Save(ctx)
	if err == nil {
		u, err = tx.User.UpdateOne(u).SetLecTeamOwnerID(u.ID).Save(ctx)
	}
	if err == nil {
		_, err = tx.File.Create().SetName(RootFolderName).SetOwnerID(u.ID).SetType(int(types.FileTypeFolder)).Save(ctx)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		_ = tx.Rollback()
		if ent.IsConstraintError(err) {
			return lookup()
		}
		return nil, err
	}
	return lookup()
}

// AssignLecTeam 保留原文件和直链编号，将已有个人目录整体迁入团队。
func AssignLecTeam(ctx context.Context, db *ent.Client, member, owner *ent.User) error {
	if member.LecTeamOwnerID != nil {
		if *member.LecTeamOwnerID != owner.ID {
			return fmt.Errorf("成员已属于其他团队")
		}
		return nil
	}
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	count, err := tx.User.Update().Where(user.ID(member.ID), user.LecTeamOwnerIDIsNil()).SetLecTeamOwnerID(owner.ID).Save(ctx)
	if err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	locked, err := tx.User.Get(ctx, member.ID)
	if err != nil {
		return err
	}
	active, err := tx.Entity.Query().Where(entity.HasFileWith(file.OwnerID(member.ID)), entity.UploadSessionIDNotNil()).Exist(ctx)
	if err != nil {
		return err
	}
	if active {
		return fmt.Errorf("请先完成或取消正在进行的上传，再初始化团队空间")
	}
	root, err := tx.File.Query().Where(file.OwnerID(member.ID), file.Name(RootFolderName), file.Not(file.HasParent())).Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return err
	}
	if root != nil {
		teamRoot, err := tx.File.Query().Where(file.OwnerID(owner.ID), file.Name(RootFolderName), file.Not(file.HasParent())).Only(ctx)
		if err != nil {
			return err
		}
		folder := fmt.Sprintf("迁入文件-%d-%s", member.ID, rand.Text()[:8])
		if _, err = tx.File.UpdateOne(root).SetName(folder).SetLecURLName(fmt.Sprintf("imported_%d_%s", member.ID, rand.Text()[:8])).SetParentID(teamRoot.ID).Save(ctx); err != nil {
			return err
		}
		// 回收站中的原恢复位置也跟随目录迁入，避免恢复到团队根目录的错误位置。
		marks, err := tx.Metadata.Query().Where(metadata.HasFileWith(file.OwnerID(member.ID)), metadata.Name(RestoreURIKey)).All(ctx)
		if err != nil {
			return err
		}
		for _, mark := range marks {
			u, err := url.Parse(mark.Value)
			if err != nil {
				return err
			}
			u.User = nil
			u.Path = path.Join("/", folder, u.Path)
			if _, err = tx.Metadata.UpdateOne(mark).SetValue(u.String()).Save(ctx); err != nil {
				return err
			}
		}
	}
	if _, err = tx.Metadata.Update().Where(metadata.HasFileWith(file.OwnerID(member.ID)), metadata.Name(TrashCollectTimeKey)).
		SetValue(strconv.FormatInt(time.Now().Add(LecTeamRetention).Unix(), 10)).Save(ctx); err != nil {
		return err
	}
	if _, err = tx.File.Update().Where(file.OwnerID(member.ID), file.Not(file.HasParent()), file.NameNEQ(RootFolderName)).SetLecDeletedAt(time.Now()).Save(ctx); err != nil {
		return err
	}
	files, err := tx.File.Query().Where(file.OwnerID(member.ID)).All(ctx)
	if err != nil {
		return err
	}
	for _, record := range files {
		// 迁移归属不能改掉照片的原始修改时间。
		if _, err = tx.File.UpdateOne(record).SetOwnerID(owner.ID).SetUpdatedAt(record.UpdatedAt).Save(ctx); err != nil {
			return err
		}
	}
	if _, err = tx.User.UpdateOneID(member.ID).SetStorage(0).Save(ctx); err != nil {
		return err
	}
	if _, err = tx.Share.Update().Where(share.HasUserWith(user.ID(member.ID))).SetUserID(owner.ID).Save(ctx); err != nil {
		return err
	}
	if _, err = tx.User.UpdateOneID(owner.ID).AddStorage(locked.Storage).Save(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

func TrashDeadline(root *ent.File) (time.Time, error) {
	if root.LecDeletedAt == nil {
		return time.Time{}, fmt.Errorf("团队图片缺少有效的回收时间，禁止提前清理")
	}
	return root.LecDeletedAt.Add(LecTeamRetention), nil
}

// CheckTeamPurge 在共同数据层拦截永久删除，后台管理入口也必须遵守回收期限。
func CheckTeamPurge(ctx context.Context, db *ent.Client, files []*ent.File) error {
	for _, item := range files {
		owner, err := db.User.Get(ctx, item.OwnerID)
		if err != nil {
			return err
		}
		if owner.LecTeamOwnerID == nil {
			continue
		}
		// 尚未完成上传的占位文件可以取消，不影响已上传的图片。
		if item.Type == int(types.FileTypeFile) && item.PrimaryEntity == 0 {
			complete, err := db.Entity.Query().Where(entity.HasFileWith(file.ID(item.ID)), entity.UploadSessionIDIsNil()).Exist(ctx)
			if err != nil {
				return err
			}
			if !complete {
				continue
			}
		}
		root, err := db.File.Query().Where(file.ID(item.ID)).WithMetadata().Only(ctx)
		if err != nil {
			return err
		}
		for root.FileChildren != 0 {
			root, err = db.File.Query().Where(file.ID(root.FileChildren)).WithMetadata().Only(ctx)
			if err != nil {
				return err
			}
		}
		if root.Name == RootFolderName {
			return fmt.Errorf("团队文件必须先移入回收站，保留 30 天")
		}
		deadline, err := TrashDeadline(root)
		if err != nil {
			return err
		}
		if time.Now().Before(deadline) {
			return fmt.Errorf("团队文件处于 30 天回收期，禁止提前彻底删除")
		}
	}
	return nil
}
