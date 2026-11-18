package explorer

import (
	"testing"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
	"github.com/stretchr/testify/require"
)

func Test上传会话按团队空间校验且拒绝跨团队写入(t *testing.T) {
	owner := &ent.User{ID: 10}
	member := &ent.User{ID: 20, LecTeamOwnerID: &owner.ID, Edges: ent.UserEdges{LecTeamOwner: owner}}
	for _, tc := range []struct {
		name    string
		user    *ent.User
		uid     int
		allowed bool
	}{
		{"成员上传团队文件", member, 10, true},
		{"拒绝其他团队会话", member, 30, false},
		{"不恢复成员个人空间", member, 20, false},
		{"独立账号兼容", &ent.User{ID: 40}, 40, true},
		{"独立账号不可写团队", &ent.User{ID: 40}, 10, false},
		{"匿名拒绝", nil, 10, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.allowed, ownsUploadSession(tc.user, &fs.UploadSession{UID: tc.uid}))
		})
	}
}
