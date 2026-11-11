package inventory

import (
	"path/filepath"
	"testing"

	"github.com/cloudreve/Cloudreve/v4/ent/enttest"
	"github.com/cloudreve/Cloudreve/v4/ent/file"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/boolset"
	"github.com/stretchr/testify/require"
)

func Test文件夹英文名校验(t *testing.T) {
	for _, name := range []string{"events", "Photos_2026", "123"} {
		require.NoError(t, ValidateFolderURLName(name, true))
	}
	for _, name := range []string{"", "中文", "a-b", "a b", "a/b", "common", "API"} {
		require.Error(t, ValidateFolderURLName(name, true), name)
	}
	require.NoError(t, ValidateFolderURLName("api", false))
}

func Test可读路径迁移和唯一绑定(t *testing.T) {
	ctx := t.Context()
	db := enttest.Open(t, "sqlite3", filepath.Join(t.TempDir(), "urls.db")+"?_fk=1")
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	policy, err := db.StoragePolicy.Create().SetName("存储").SetType(types.PolicyTypeLocal).Save(ctx)
	require.NoError(t, err)
	group, err := db.Group.Create().SetName("组").SetPermissions(&boolset.BooleanSet{}).SetStoragePolicyID(policy.ID).Save(ctx)
	require.NoError(t, err)
	owner, err := db.User.Create().SetEmail("team@sso.invalid").SetNick("团队").SetGroupID(group.ID).Save(ctx)
	require.NoError(t, err)
	owner, err = db.User.UpdateOne(owner).SetLecTeamOwnerID(owner.ID).Save(ctx)
	require.NoError(t, err)
	root, err := db.File.Create().SetName(RootFolderName).SetOwnerID(owner.ID).SetType(int(types.FileTypeFolder)).Save(ctx)
	require.NoError(t, err)
	fc := NewFileClient(db, "sqlite3", nil)
	folder, err := fc.CreateFolder(ctx, root, &CreateFolderParameters{Owner: owner.ID, Name: "活动照片", URLName: "events"})
	require.NoError(t, err)
	require.Equal(t, "活动照片", folder.Name)
	_, err = fc.CreateFolder(ctx, root, &CreateFolderParameters{Owner: owner.ID, Name: "另一活动", URLName: "events"})
	require.Error(t, err)
	nested, err := fc.CreateFolder(ctx, folder, &CreateFolderParameters{Owner: owner.ID, Name: "今年", URLName: "2026"})
	require.NoError(t, err)
	image, err := db.File.Create().SetName("照片 #1.png").SetOwnerID(owner.ID).SetType(int(types.FileTypeFile)).SetParent(nested).Save(ctx)
	require.NoError(t, err)
	link, err := EnsureReadableLink(ctx, db, image, 0)
	require.NoError(t, err)
	require.Equal(t, "events/2026/照片 #1.png", *link.PublicPath)
	again, err := EnsureReadableLink(ctx, db, image, 0)
	require.NoError(t, err)
	require.Equal(t, link.ID, again.ID)
	// 模拟旧图片进入回收站，同名新图片不能接管原路径。
	_, err = db.File.UpdateOne(image).SetName("回收编号").Save(ctx)
	require.NoError(t, err)
	replacement, err := db.File.Create().SetName(image.Name).SetOwnerID(owner.ID).SetType(int(types.FileTypeFile)).SetParent(nested).Save(ctx)
	require.NoError(t, err)
	_, err = EnsureReadableLink(ctx, db, replacement, 0)
	require.Error(t, err)
	actual, err := db.DirectLink.Get(ctx, link.ID)
	require.NoError(t, err)
	require.Equal(t, image.ID, actual.FileID)
	legacy, err := db.File.Create().SetName("原有中文目录").SetOwnerID(owner.ID).SetType(int(types.FileTypeFolder)).SetParent(root).Save(ctx)
	require.NoError(t, err)
	rootImage, err := db.File.Create().SetName("根图片.png").SetOwnerID(owner.ID).SetType(int(types.FileTypeFile)).SetParent(root).Save(ctx)
	require.NoError(t, err)
	_, err = db.DirectLink.Create().SetFile(rootImage).SetName(rootImage.Name).SetDownloads(0).SetSpeed(0).Save(ctx)
	require.NoError(t, err)
	require.NoError(t, migrateReadablePaths(ctx, db))
	require.NoError(t, migrateReadablePaths(ctx, db))
	legacy, err = db.File.Get(ctx, legacy.ID)
	require.NoError(t, err)
	require.Equal(t, "原有中文目录", legacy.Name)
	require.NotNil(t, legacy.LecURLName)
	link, err = EnsureReadableLink(ctx, db, rootImage, 0)
	require.NoError(t, err)
	require.Equal(t, "common/根图片.png", *link.PublicPath)
	require.Equal(t, 4, db.File.Query().Where(file.OwnerID(owner.ID), file.Type(int(types.FileTypeFolder))).CountX(ctx))
}
