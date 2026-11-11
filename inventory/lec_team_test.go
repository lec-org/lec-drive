package inventory

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudreve/Cloudreve/v4/ent/enttest"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/boolset"
	"github.com/stretchr/testify/require"
)

func Test个人文件原位迁入团队且重复执行不重复计量(t *testing.T) {
	ctx := t.Context()
	db := enttest.Open(t, "sqlite3", filepath.Join(t.TempDir(), "migration.db")+"?_fk=1")
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	policy, err := db.StoragePolicy.Create().SetName("测试").SetType(types.PolicyTypeLocal).Save(ctx)
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		_, err := db.Group.Create().SetName("测试组").SetPermissions(&boolset.BooleanSet{}).SetMaxStorage(1 << 40).SetStoragePolicyID(policy.ID).Save(ctx)
		require.NoError(t, err)
	}
	member, err := db.User.Create().SetEmail("member@sso.invalid").SetNick("原成员").SetGroupID(2).SetStorage(10).Save(ctx)
	require.NoError(t, err)
	root, err := db.File.Create().SetOwnerID(member.ID).SetName(RootFolderName).SetType(int(types.FileTypeFolder)).Save(ctx)
	require.NoError(t, err)
	modified := time.Now().Add(-time.Hour)
	image, err := db.File.Create().SetOwnerID(member.ID).SetName("original.png").SetType(int(types.FileTypeFile)).SetSize(10).SetParent(root).SetUpdatedAt(modified).Save(ctx)
	require.NoError(t, err)
	entity, err := db.Entity.Create().SetType(int(types.EntityTypeVersion)).SetSource("original.png").SetSize(10).
		SetStoragePolicyID(policy.ID).SetUser(member).AddFile(image).Save(ctx)
	require.NoError(t, err)
	link, err := db.DirectLink.Create().SetName("original.png").SetFile(image).SetDownloads(0).SetSpeed(0).Save(ctx)
	require.NoError(t, err)
	shared, err := db.Share.Create().SetFile(image).SetUser(member).Save(ctx)
	require.NoError(t, err)
	owner, err := EnsureLecTeamOwner(ctx, db, "https://sso.example/oidc", "lec-studio")
	require.NoError(t, err)
	require.False(t, owner.Edges.Group.Permissions.Enabled(int(types.GroupPermissionIsAdmin)), "资产账号不能带管理员权限")
	require.Error(t, NewUserClient(db).Delete(ctx, owner.ID), "不能删除团队资产账号")
	require.NoError(t, AssignLecTeam(ctx, db, member, owner))
	require.NoError(t, AssignLecTeam(ctx, db, member, owner))
	actual, err := db.File.Get(ctx, image.ID)
	require.NoError(t, err)
	require.Equal(t, owner.ID, actual.OwnerID)
	require.Equal(t, "original.png", actual.Name)
	require.WithinDuration(t, modified, actual.UpdatedAt, time.Second)
	actualLink, err := db.DirectLink.Query().WithFile().Only(ctx)
	require.NoError(t, err)
	require.Equal(t, link.ID, actualLink.ID)
	require.Equal(t, image.ID, actualLink.Edges.File.ID)
	actualShare, err := db.Share.Query().WithUser().Only(ctx)
	require.NoError(t, err)
	require.Equal(t, shared.ID, actualShare.ID)
	require.Equal(t, owner.ID, actualShare.Edges.User.ID)
	actualOwner, err := db.User.Get(ctx, owner.ID)
	require.NoError(t, err)
	require.EqualValues(t, 10, actualOwner.Storage)
	actualMember, err := db.User.Get(ctx, member.ID)
	require.NoError(t, err)
	require.Zero(t, actualMember.Storage)
	require.Equal(t, owner.ID, *actualMember.LecTeamOwnerID)
	actualEntity, err := db.Entity.Get(ctx, entity.ID)
	require.NoError(t, err)
	require.Equal(t, member.ID, actualEntity.CreatedBy, "迁移后仍保留实际上传者")
}
