package inventory

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"entgo.io/ent/dialect/sql"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/directlink"
	"github.com/cloudreve/Cloudreve/v4/ent/file"
	"github.com/cloudreve/Cloudreve/v4/ent/schema"
	"github.com/cloudreve/Cloudreve/v4/ent/user"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
)

var folderURLName = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)
var ErrPublicFileInTrash = errors.New("回收站文件请使用删除前的链接")

// 应用页面和静态资源拥有固定前缀，不能被顶层文件夹占用。
var reservedURLNames = []string{"common", "api", "admin", "assets", "static", "locales", "session", "home", "callback", "profile", "settings", "tasks", "downloads", "shares", "connect", "dav", "f", "s"}

func IsApplicationURLName(name string) bool {
	return slices.Contains(reservedURLNames[1:], strings.ToLower(name))
}

func ValidateFolderURLName(name string, topLevel bool) error {
	if !folderURLName.MatchString(name) {
		return fmt.Errorf("文件夹英文名必填，只允许英文字母、数字和下划线，最多 64 个字符")
	}
	if topLevel && slices.Contains(reservedURLNames, strings.ToLower(name)) {
		return fmt.Errorf("英文名 %q 为系统保留名称，请换一个名称", name)
	}
	return nil
}

func readableFilePath(ctx context.Context, db *ent.Client, target *ent.File) (string, error) {
	parts := []string{target.Name}
	current := target
	for current.FileChildren != 0 {
		parent, err := db.File.Get(ctx, current.FileChildren)
		if err != nil {
			return "", err
		}
		if parent.OwnerID != target.OwnerID {
			return "", fmt.Errorf("目录归属不一致")
		}
		if parent.FileChildren != 0 {
			if parent.LecURLName == nil || !folderURLName.MatchString(*parent.LecURLName) {
				return "", fmt.Errorf("请先为文件夹填写有效的英文名")
			}
			parts = append(parts, *parent.LecURLName)
		}
		current = parent
	}
	if current.Name != RootFolderName {
		return "", ErrPublicFileInTrash
	}
	if len(parts) == 1 {
		parts = append(parts, "common")
	}
	slices.Reverse(parts)
	result := strings.Join(parts, "/")
	if len(result) > 2048 {
		return "", fmt.Errorf("图片访问路径过长")
	}
	return result, nil
}

// 链接路径固定绑定文件编号，删除后的保留期内不能被同名新图片接管。
func EnsureReadableLink(ctx context.Context, db *ent.Client, target *ent.File, speed int) (*ent.DirectLink, error) {
	path, err := readableFilePath(ctx, db, target)
	if err != nil {
		return nil, err
	}
	_, err = db.DirectLink.Create().SetPublicPath(path).SetFileID(target.ID).SetName(target.Name).SetDownloads(0).SetSpeed(speed).
		OnConflict(sql.ConflictColumns(directlink.FieldPublicPath)).Ignore().ID(ctx)
	if err != nil {
		return nil, err
	}
	link, err := db.DirectLink.Query().Where(directlink.PublicPath(path)).Only(ctx)
	if err != nil {
		return nil, err
	}
	if link.FileID != target.ID {
		return nil, serializer.NewError(serializer.CodeConflict, "该图片路径已被占用或仍在删除保留期内，请更换文件名", nil)
	}
	return link, nil
}

// 首次切换时给测试目录补齐英文访问名，保留原中文显示名和图片本身。
func migrateReadablePaths(ctx context.Context, db *ent.Client) error {
	folders, err := db.File.Query().Where(file.Type(int(types.FileTypeFolder)), file.NameNEQ(RootFolderName), file.LecURLNameIsNil()).Order(ent.Asc(file.FieldID)).All(ctx)
	if err != nil {
		return err
	}
	for _, folder := range folders {
		top := folder.FileChildren == 0
		if !top {
			parent, err := db.File.Get(ctx, folder.FileChildren)
			if err != nil {
				return err
			}
			top = parent.FileChildren == 0 && parent.Name == RootFolderName
		}
		candidate := folder.Name
		if ValidateFolderURLName(candidate, top) != nil {
			candidate = "folder_" + strconv.Itoa(folder.ID)
		}
		base := candidate
		for i := 1; ; i++ {
			q := db.File.Query().Where(file.OwnerID(folder.OwnerID), file.LecURLName(candidate))
			if folder.FileChildren == 0 {
				q.Where(file.Not(file.HasParent()))
			} else {
				q.Where(file.FileChildren(folder.FileChildren))
			}
			exists, err := q.Exist(ctx)
			if err != nil {
				return err
			}
			if !exists {
				break
			}
			candidate = base + "_" + strconv.Itoa(i)
		}
		if _, err = db.File.UpdateOne(folder).SetLecURLName(candidate).SetUpdatedAt(folder.UpdatedAt).Save(ctx); err != nil {
			return err
		}
	}
	// 项目尚未上线，直接移除旧短码记录；对外只生成新路径。
	if _, err = db.DirectLink.Delete().Where(directlink.PublicPathIsNil()).Exec(schema.SkipSoftDelete(ctx)); err != nil {
		return err
	}
	files, err := db.File.Query().Where(file.Type(int(types.FileTypeFile)), file.PrimaryEntityGT(0), file.HasOwnerWith(user.LecTeamOwnerIDNotNil())).All(ctx)
	if err != nil {
		return err
	}
	for _, item := range files {
		if _, err = EnsureReadableLink(ctx, db, item, 0); errors.Is(err, ErrPublicFileInTrash) {
			continue
		} else if err != nil {
			return err
		}
	}
	return nil
}
