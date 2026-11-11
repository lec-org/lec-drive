package user

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	userent "github.com/cloudreve/Cloudreve/v4/ent/user"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/pkg/lecsso"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/pkg/util"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
)

const lecCookie = "lecdrive_oidc"
const lecChallengePrefix = "lec_oidc_"

type lecChallenge struct{ Nonce, Verifier string }

// LecSSOLogin 以一次性 state、nonce 和 PKCE 发起统一登录。
func LecSSOLogin(c *gin.Context) {
	config, err := lecsso.ReadConfig()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, serializer.Err(c, err))
		return
	}
	client, err := lecsso.New(c, config)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, serializer.Err(c, err))
		return
	}
	state, nonce, verifier := oauth2.GenerateVerifier(), oauth2.GenerateVerifier(), oauth2.GenerateVerifier()
	challenge, _ := json.Marshal(lecChallenge{Nonce: nonce, Verifier: verifier})
	if err := dependency.FromContext(c).KV().Set(lecChallengePrefix+state, string(challenge), 600); err != nil {
		c.JSON(http.StatusServiceUnavailable, serializer.Err(c, errors.New("无法创建登录会话")))
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(lecCookie, state, 600, "/api/v4/session/lec-sso", "", strings.HasPrefix(config.Origin, "https://"), true)
	c.Redirect(http.StatusFound, client.LoginURL(state, nonce, verifier))
}

// LecSSOComplete 校验外部身份后签发原有 Drive 会话，Token 不放入跳转 URL。
func LecSSOComplete(c *gin.Context) {
	var request struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}
	state, _ := c.Cookie(lecCookie)
	if err := c.ShouldBindJSON(&request); err != nil || state == "" || request.Code == "" || subtle.ConstantTimeCompare([]byte(state), []byte(request.State)) != 1 {
		c.JSON(http.StatusUnauthorized, serializer.Err(c, errors.New("登录会话不匹配，请重新登录")))
		return
	}
	dep := dependency.FromContext(c)
	raw, ok := dep.KV().Get(lecChallengePrefix + state)
	if !ok {
		c.JSON(http.StatusUnauthorized, serializer.Err(c, errors.New("登录会话已过期")))
		return
	}
	if err := dep.KV().Delete(lecChallengePrefix + state); err != nil {
		c.JSON(http.StatusServiceUnavailable, serializer.Err(c, errors.New("无法消费登录会话")))
		return
	}
	c.SetCookie(lecCookie, "", -1, "/api/v4/session/lec-sso", "", false, true)
	var challenge lecChallenge
	serialized, ok := raw.(string)
	if !ok || json.Unmarshal([]byte(serialized), &challenge) != nil {
		c.JSON(http.StatusUnauthorized, serializer.Err(c, errors.New("登录会话无效")))
		return
	}
	config, err := lecsso.ReadConfig()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, serializer.Err(c, err))
		return
	}
	client, err := lecsso.New(c, config)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, serializer.Err(c, err))
		return
	}
	claims, err := client.Complete(c, request.Code, challenge.Nonce, challenge.Verifier)
	if err != nil {
		c.JSON(http.StatusUnauthorized, serializer.Err(c, err))
		return
	}
	loginUser, err := upsertLecUser(c, config, claims)
	if err != nil {
		dep.Logger().Warning("LecSSO 用户同步失败: %s", err)
		c.JSON(http.StatusInternalServerError, serializer.Err(c, errors.New("无法同步 LecSSO 用户")))
		return
	}
	util.WithValue(c, inventory.UserCtx{}, loginUser)
	response, err := IssueToken(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, serializer.Err(c, err))
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, serializer.Response{Data: response})
}

func upsertLecUser(c *gin.Context, config lecsso.Config, claims *lecsso.Claims) (*ent.User, error) {
	dep := dependency.FromContext(c)
	query := dep.DBClient().User.Query().Where(userent.LecSSOIssuerEQ(config.Issuer), userent.LecSSOSubjectEQ(claims.Subject))
	u, err := query.Only(c)
	groupID := dep.SettingProvider().DefaultGroup(c)
	if groupID == 1 {
		groupID = 2
	}
	if slices.Contains(config.AdminSubjects, claims.Subject) {
		groupID = 1
	}
	if ent.IsNotFound(err) {
		uc, tx, ctx, txErr := inventory.WithTx(c, dep.UserClient())
		if txErr != nil {
			return nil, txErr
		}
		defer inventory.Rollback(tx)
		identity := sha256.Sum256([]byte(config.Issuer + "\x00" + claims.Subject))
		u, err = uc.Create(ctx, &inventory.NewUserArgs{
			Email: hex.EncodeToString(identity[:]) + "@sso.invalid", Status: userent.StatusActive,
			GroupID: groupID, Language: "zh-CN",
		})
		if err == nil {
			// 覆盖上游“首个注册者即管理员”的规则，管理员只由显式 subject 白名单授予。
			u, err = uc.GetClient().User.UpdateOne(u).SetLecSSOIssuer(config.Issuer).SetLecSSOSubject(claims.Subject).SetGroupID(groupID).Save(ctx)
		}
		if err != nil {
			return nil, err
		}
		if err = inventory.Commit(tx); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if u.Status != userent.StatusActive {
		return nil, errors.New("当前 Drive 账号不可用")
	}
	name := strings.TrimSpace(claims.Name)
	if name == "" {
		name = "LEC 成员"
	}
	name = string([]rune(name)[:min(len([]rune(name)), 100)])
	if _, err = dep.DBClient().User.UpdateOneID(u.ID).SetNick(name).SetAvatar(claims.Picture).SetGroupID(groupID).Save(c); err != nil {
		return nil, err
	}
	return dep.UserClient().GetLoginUserByID(c, u.ID)
}
