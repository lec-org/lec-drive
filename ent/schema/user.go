package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		// 独立保存外部身份，避免按邮箱合并导致账号被错误关联。
		field.String("lec_sso_issuer").Optional().Nillable(),
		field.String("lec_sso_subject").Optional().Nillable(),
		field.Int("lec_team_owner_id").Optional().Nillable(),
		field.String("email").
			MaxLen(100).
			Unique(),
		field.String("nick").
			MaxLen(100),
		field.String("password").
			Optional().
			Sensitive(),
		field.Enum("status").
			Values("active", "inactive", "manual_banned", "sys_banned").
			Default("active"),
		field.Int64("storage").
			Default(0),
		field.String("two_factor_secret").
			Sensitive().
			Optional(),
		field.String("avatar").
			Optional(),
		field.JSON("settings", &types.UserSetting{}).
			Default(&types.UserSetting{}).
			Optional(),
		field.Int("group_users"),
	}
}

func (User) Indexes() []ent.Index {
	return []ent.Index{index.Fields("lec_sso_issuer", "lec_sso_subject").Unique()}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("lec_team_members", User.Type).From("lec_team_owner").Unique().Field("lec_team_owner_id"),
		edge.From("group", Group.Type).
			Ref("users").
			Field("group_users").
			Unique().
			Required(),
		edge.To("files", File.Type),
		edge.To("dav_accounts", DavAccount.Type),
		edge.To("shares", Share.Type),
		edge.To("passkey", Passkey.Type),
		edge.To("tasks", Task.Type),
		edge.To("fsevents", FsEvent.Type),
		edge.To("entities", Entity.Type),
		edge.To("oauth_grants", OAuthGrant.Type),
	}
}

func (User) Mixin() []ent.Mixin {
	return []ent.Mixin{
		CommonMixin{},
	}
}
