package manager

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/enttest"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/boolset"
	"github.com/cloudreve/Cloudreve/v4/pkg/cache"
	"github.com/cloudreve/Cloudreve/v4/pkg/conf"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
	"github.com/cloudreve/Cloudreve/v4/pkg/logging"
	"github.com/cloudreve/Cloudreve/v4/pkg/setting"
	"github.com/stretchr/testify/require"
)

func Test团队成员权限与三十天直链保留(t *testing.T) {
	ctx := t.Context()
	db := enttest.Open(t, "sqlite3", filepath.Join(t.TempDir(), "team.db")+"?_fk=1")
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	policy, err := db.StoragePolicy.Create().SetName("测试存储").SetType(types.PolicyTypeLocal).Save(ctx)
	require.NoError(t, err)
	for _, admin := range []bool{true, false} {
		permissions := &boolset.BooleanSet{}
		boolset.Set(types.GroupPermissionIsAdmin, admin, permissions)
		_, err = db.Group.Create().SetName("测试组").SetPermissions(permissions).SetMaxStorage(1 << 40).
			SetSettings(&types.GroupSetting{SourceBatchSize: 100, TrashRetention: 7 * 24 * 3600}).SetStoragePolicyID(policy.ID).Save(ctx)
		require.NoError(t, err)
	}
	owner, err := inventory.EnsureLecTeamOwner(ctx, db, "https://sso.example/oidc", "lec-studio")
	require.NoError(t, err)
	uc := inventory.NewUserClient(db)
	createMember := func(name string, group int) *ent.User {
		u, err := db.User.Create().SetEmail(name + "@sso.invalid").SetNick(name).SetGroupID(group).
			SetLecSSOIssuer("https://sso.example/oidc").SetLecSSOSubject(name).SetLecTeamOwnerID(owner.ID).Save(ctx)
		require.NoError(t, err)
		u, err = uc.GetLoginUserByID(ctx, u.ID)
		require.NoError(t, err)
		return u
	}
	member, admin := createMember("成员", 2), createMember("管理员", 1)
	logger := logging.NewConsoleLogger(logging.LevelError)
	config, err := conf.NewIniConfigProvider(filepath.Join(t.TempDir(), "test.ini"), logger)
	require.NoError(t, err)
	hasher, err := hashid.New("team-test")
	require.NoError(t, err)
	dep := dependency.NewDependency(dependency.WithDbClient(db), dependency.WithConfigProvider(config), dependency.WithLogger(logger),
		dependency.WithHashIDEncoder(hasher), dependency.WithKV(cache.NewMemoStore("", logger)),
		dependency.WithSettingProvider(setting.NewProvider(setting.NewDbDefaultStore(nil))))
	ctx = context.WithValue(ctx, dependency.DepCtx{}, dep)
	uri := func(path string) *fs.URI { u, err := fs.NewUriFromString(path); require.NoError(t, err); return u }
	memberFS, adminFS := NewFileManager(dep, member).(*manager), NewFileManager(dep, admin).(*manager)
	t.Cleanup(memberFS.Recycle)
	t.Cleanup(adminFS.Recycle)
	root, err := dep.FileClient().Root(ctx, owner)
	require.NoError(t, err)
	image, err := db.File.Create().SetName("photo.png").SetType(int(types.FileTypeFile)).SetOwnerID(owner.ID).SetParent(root).Save(ctx)
	require.NoError(t, err)
	entity, err := db.Entity.Create().SetType(int(types.EntityTypeVersion)).SetSource("photo.png").SetSize(10).
		SetStoragePolicyID(policy.ID).SetUser(member).AddFile(image).Save(ctx)
	require.NoError(t, err)
	image, err = db.File.UpdateOne(image).SetPrimaryEntity(entity.ID).SetSize(10).Save(ctx)
	require.NoError(t, err)
	link, err := inventory.EnsureReadableLink(ctx, db, image, 0)
	require.NoError(t, err)
	loadLink := func(target *ent.DirectLink) *ent.DirectLink {
		c := context.WithValue(ctx, inventory.LoadDirectLinkFile{}, true)
		c = context.WithValue(c, inventory.LoadFileUser{}, true)
		c = context.WithValue(c, inventory.LoadUserGroup{}, true)
		c = context.WithValue(c, inventory.LoadFileEntity{}, true)
		loaded, err := dep.DirectLinkClient().GetByNameID(c, target.ID, target.Name)
		require.NoError(t, err)
		return loaded
	}
	path := uri("cloudreve://my/photo.png")
	a, err := memberFS.Get(ctx, path)
	require.NoError(t, err)
	b, err := adminFS.Get(ctx, path)
	require.NoError(t, err)
	require.Equal(t, a.ID(), b.ID(), "成员共用同一份团队文件")
	_, err = memberFS.Get(ctx, uri("cloudreve://"+hashid.EncodeUserID(hasher, member.ID)+"@my"))
	require.Error(t, err, "不能通过显式用户编号恢复个人空间")
	require.Error(t, memberFS.Delete(ctx, []*fs.URI{path}))
	require.Error(t, memberFS.SoftDelete(ctx, path))
	require.Error(t, memberFS.Delete(ctx, []*fs.URI{path}, fs.WithSkipSoftDelete(true)))
	require.Error(t, adminFS.Delete(ctx, []*fs.URI{path}, fs.WithSkipSoftDelete(true)))
	require.Error(t, adminFS.RecycleEntities(ctx, true, entity.ID))
	require.Error(t, inventory.CheckTeamPurge(ctx, db, []*ent.File{image}), "后台管理也不能直接彻底删除活跃图片")
	// 引用异常的已发布图片也不能被当成未完成上传直接清理。
	require.NoError(t, db.File.UpdateOneID(image.ID).ClearEntities().Exec(ctx))
	require.Error(t, inventory.CheckTeamPurge(ctx, db, []*ent.File{image}))
	require.NoError(t, db.File.UpdateOneID(image.ID).AddEntities(entity).Exec(ctx))
	require.NoError(t, adminFS.Delete(ctx, []*fs.URI{path}))
	deleted, err := db.File.Get(ctx, image.ID)
	require.NoError(t, err)
	require.NotNil(t, deleted.LecDeletedAt)
	before := db.File.Query().CountX(ctx)
	_, err = memberFS.Create(ctx, path, types.FileTypeFile)
	require.Error(t, err, "回收保留期内不能复用原图片路径")
	require.Equal(t, before, db.File.Query().CountX(ctx), "冲突上传不能留下占位文件")
	deadline, err := inventory.TrashDeadline(deleted)
	require.NoError(t, err)
	require.WithinDuration(t, deleted.LecDeletedAt.Add(30*24*time.Hour), deadline, time.Second)
	_, err = adminFS.fs.GetFileFromDirectLink(ctx, loadLink(link))
	require.NoError(t, err, "管理员删除后原直链必须继续可用")
	require.Error(t, inventory.CheckTeamPurge(ctx, db, []*ent.File{deleted}))
	require.Error(t, adminFS.EmptyTrash(ctx))
	trash := uri("cloudreve://trash/" + deleted.Name)
	require.Error(t, memberFS.Restore(ctx, trash))
	require.NoError(t, adminFS.Restore(ctx, trash))
	restored, err := db.File.Get(ctx, image.ID)
	require.NoError(t, err)
	require.Nil(t, restored.LecDeletedAt)
	require.Equal(t, "photo.png", restored.Name)
	_, err = adminFS.fs.GetFileFromDirectLink(ctx, loadLink(link))
	require.NoError(t, err, "恢复后仍使用原文件编号和直链")
	// 删除整个目录时，内部图片沿用目录的回收期。
	folder, err := adminFS.Create(ctx, uri("cloudreve://my/folder"), types.FileTypeFolder)
	require.NoError(t, err)
	child, err := db.File.Create().SetName("child.png").SetOwnerID(owner.ID).SetType(int(types.FileTypeFile)).
		SetParentID(folder.ID()).SetPrimaryEntity(entity.ID).AddEntities(entity).Save(ctx)
	require.NoError(t, err)
	childLink, err := inventory.EnsureReadableLink(ctx, db, child, 0)
	require.NoError(t, err)
	adminFS = NewFileManager(dep, admin).(*manager)
	require.NoError(t, adminFS.Delete(ctx, []*fs.URI{uri("cloudreve://my/folder")}))
	_, err = adminFS.fs.GetFileFromDirectLink(ctx, loadLink(childLink))
	require.NoError(t, err)
	require.Error(t, inventory.CheckTeamPurge(ctx, db, []*ent.File{child}))
	_, err = db.File.UpdateOneID(folder.ID()).SetLecDeletedAt(time.Now().Add(-31 * 24 * time.Hour)).Save(ctx)
	require.NoError(t, err)
	_, err = adminFS.fs.GetFileFromDirectLink(ctx, loadLink(childLink))
	require.Error(t, err)
	require.NoError(t, inventory.CheckTeamPurge(ctx, db, []*ent.File{child}))
	// 新建管理器丢弃目录缓存，模拟新的 HTTP 请求。
	adminFS = NewFileManager(dep, admin).(*manager)
	require.NoError(t, adminFS.Delete(ctx, []*fs.URI{path}))
	deleted, err = db.File.UpdateOneID(image.ID).SetLecDeletedAt(time.Now().Add(-31 * 24 * time.Hour)).Save(ctx)
	require.NoError(t, err)
	adminFS = NewFileManager(dep, admin).(*manager)
	require.Error(t, adminFS.Restore(ctx, uri("cloudreve://trash/"+deleted.Name)), "超过 30 天不能恢复")
	_, err = adminFS.fs.GetFileFromDirectLink(ctx, loadLink(link))
	require.Error(t, err, "超过期限的回收站图片不再提供直链")
	require.NoError(t, inventory.CheckTeamPurge(ctx, db, []*ent.File{deleted}), "到期后允许系统清理")
}
