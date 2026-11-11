package s3

import (
	"net/url"
	"testing"
	"time"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/driver"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
	"github.com/stretchr/testify/require"
)

func Test公开图片下载保留响应头与签名(t *testing.T) {
	for _, private := range []bool{false, true} {
		policy := &ent.StoragePolicy{Server: "https://storage.example", BucketName: "photos", AccessKey: "test-key", SecretKey: "test-secret", IsPrivate: private, Settings: &types.PolicySetting{Region: "us-east-1", S3ForcePathStyle: true}}
		d, err := New(t.Context(), policy, nil, nil, nil, nil)
		require.NoError(t, err)
		for _, download := range []bool{false, true} {
			expire := time.Now().Add(time.Hour)
			raw, err := d.Source(t.Context(), fs.NewEntity(&ent.Entity{Source: "image.png"}), &driver.GetSourceArgs{Expire: &expire, IsDownload: download, DisplayName: "照片.png"})
			require.NoError(t, err)
			u, err := url.Parse(raw)
			require.NoError(t, err)
			require.Equal(t, private || download, u.Query().Has("X-Amz-Signature"))
			if download {
				require.Contains(t, u.Query().Get("response-content-disposition"), "attachment")
			} else {
				require.Empty(t, u.Query().Get("response-content-disposition"))
			}
		}
	}
}
