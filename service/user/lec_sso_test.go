package user

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent/enttest"
	"github.com/cloudreve/Cloudreve/v4/pkg/boolset"
	"github.com/cloudreve/Cloudreve/v4/pkg/cache"
	"github.com/cloudreve/Cloudreve/v4/pkg/lecsso"
	"github.com/cloudreve/Cloudreve/v4/pkg/logging"
	"github.com/cloudreve/Cloudreve/v4/pkg/setting"
	"github.com/cloudreve/Cloudreve/v4/pkg/util"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func Test登录回调拒绝缺失或不匹配的State(t *testing.T) {
	for _, cookie := range []string{"", "expected"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/api/v4/session/lec-sso/complete", strings.NewReader(`{"code":"code","state":"wrong"}`))
		if cookie != "" {
			c.Request.Header.Set("Cookie", lecCookie+"="+cookie)
		}
		LecSSOComplete(c)
		require.Equal(t, 401, c.Writer.Status())
	}
}

func Test外部身份隔离与显式管理员权限(t *testing.T) {
	client := enttest.Open(t, "sqlite3", filepath.Join(t.TempDir(), "identity.db"))
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	logger := logging.NewConsoleLogger(logging.LevelError)
	kv := cache.NewMemoStore("", logger)
	for _, name := range []string{"管理员", "成员"} {
		_, err := client.Group.Create().SetName(name).SetPermissions(&boolset.BooleanSet{}).Save(t.Context())
		require.NoError(t, err)
	}
	dep := dependency.NewDependency(dependency.WithDbClient(client), dependency.WithLogger(logger), dependency.WithKV(kv),
		dependency.WithSettingProvider(setting.NewProvider(setting.NewDbDefaultStore(nil))))
	c, engine := gin.CreateTestContext(httptest.NewRecorder())
	engine.ContextWithFallback = true
	c.Request = httptest.NewRequest("GET", "/", nil)
	util.WithValue(c, dependency.DepCtx{}, dep)
	config := lecsso.Config{Issuer: "https://sso.example.org/oidc"}
	claims := &lecsso.Claims{Subject: "member", Name: "测试成员"}
	first, err := upsertLecUser(c, config, claims)
	require.NoError(t, err)
	require.NotNil(t, first.LecTeamOwnerID)
	require.NotEqual(t, first.ID, *first.LecTeamOwnerID)
	require.Equal(t, 2, first.GroupUsers, "首位 SSO 用户不能自动成为管理员")
	second, err := upsertLecUser(c, config, &lecsso.Claims{Subject: "second-member", Name: "另一成员"})
	require.NoError(t, err)
	require.Equal(t, first.LecTeamOwnerID, second.LecTeamOwnerID, "同一组织成员共用资产归属")
	require.NotEqual(t, first.ID, second.ID, "成员身份不能合并")
	claims.Name = "更新姓名"
	same, err := upsertLecUser(c, config, claims)
	require.NoError(t, err)
	require.Equal(t, first.ID, same.ID)
	require.Equal(t, "更新姓名", same.Nick)
	config.Issuer = "https://other.example.org/oidc"
	other, err := upsertLecUser(c, config, claims)
	require.NoError(t, err)
	require.NotEqual(t, first.ID, other.ID, "不同发行方的同名 subject 必须隔离")
	require.NotEqual(t, first.LecTeamOwnerID, other.LecTeamOwnerID, "不同发行方的团队资产必须隔离")
	config.AdminSubjects = []string{"member"}
	admin, err := upsertLecUser(c, config, claims)
	require.NoError(t, err)
	require.Equal(t, 1, admin.GroupUsers)
	config.AdminSubjects = nil
	member, err := upsertLecUser(c, config, claims)
	require.NoError(t, err)
	require.Equal(t, 2, member.GroupUsers, "撤销显式授权后应同步回普通用户组")
	_, err = client.User.UpdateOneID(member.ID).SetStatus("manual_banned").Save(t.Context())
	require.NoError(t, err)
	_, err = upsertLecUser(c, config, claims)
	require.Error(t, err, "SSO 登录不能绕过 Drive 封禁")
}
