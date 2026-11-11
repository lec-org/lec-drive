package routes

import (
	"github.com/stretchr/testify/require"
	"net/url"
	"testing"
)

func Test可读链接编码文件名(t *testing.T) {
	base, err := url.Parse("https://drive.example")
	require.NoError(t, err)
	require.Equal(t, "https://drive.example/events/2026/%E7%85%A7%E7%89%87%20%231.png", ReadableDirectLink(base, "events/2026/照片 #1.png").String())
}
