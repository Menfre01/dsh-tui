package tui

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCheckLatestRelease 验证 302 重定向解析与版本比较:
// 新版 → UpdateAvailable=true;同版本 → false;开发版跳过。
func TestCheckLatestRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://github.com/Menfre01/dsh-tui/releases/tag/v1.2.3", http.StatusFound)
	}))
	defer srv.Close()

	info, err := checkLatestRelease(context.Background(), srv.URL+"/releases/latest", "v1.0.0")
	if err != nil {
		t.Fatalf("checkLatestRelease: %v", err)
	}
	if info.LatestVersion != "v1.2.3" {
		t.Fatalf("LatestVersion = %q, want v1.2.3", info.LatestVersion)
	}
	if !info.UpdateAvailable {
		t.Fatal("应检测到更新(v1.0.0 → v1.2.3)")
	}

	// 同版本 → 无更新
	info, err = checkLatestRelease(context.Background(), srv.URL, "v1.2.3")
	if err != nil {
		t.Fatalf("checkLatestRelease: %v", err)
	}
	if info.UpdateAvailable {
		t.Fatal("同版本不应提示更新")
	}

	// 开发版跳过
	info, err = CheckForUpdate(context.Background(), "dev")
	if err != nil || info != nil {
		t.Fatalf("dev 版本应跳过检查: info=%v err=%v", info, err)
	}
}

// TestBuildDownloadURL 验证平台资产命名与 release 命名一致。
func TestBuildDownloadURL(t *testing.T) {
	url := BuildDownloadURL()
	if !strings.Contains(url, "Menfre01/dsh-tui/releases/latest/download/dsh-tui_") {
		t.Fatalf("URL 格式异常: %s", url)
	}
	if !strings.HasSuffix(url, ".tar.gz") && !strings.HasSuffix(url, ".zip") {
		t.Fatalf("URL 应以 .tar.gz/.zip 结尾: %s", url)
	}
}

// TestSelfUpdate 验证 tar.gz 下载 → 解压 → 替换当前二进制,
// 替换后新内容生效且旧备份被清理。
func TestSelfUpdate(t *testing.T) {
	// 构造一个 tar.gz 内含 "dsh-tui" 二进制
	payload := []byte("new-binary-content-v2")
	tmpDir := t.TempDir()
	tarPath := filepath.Join(tmpDir, "dsh-tui.tar.gz")

	f, err := os.Create(tarPath)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{
		Name: "dsh-tui", Mode: 0o755, Size: int64(len(payload)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(payload); err != nil {
		t.Fatal(err)
	}
	_ = tw.Close()
	_ = gz.Close()
	_ = f.Close()

	// 下载服务器
	srv := httptest.NewServer(http.FileServer(http.Dir(tmpDir)))
	defer srv.Close()

	// 当前二进制(旧内容)
	current := filepath.Join(tmpDir, "bin", "dsh-tui")
	if err := os.MkdirAll(filepath.Dir(current), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(current, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	downloadURL := srv.URL + "/dsh-tui.tar.gz"
	if err := SelfUpdate(context.Background(), current, downloadURL, nil); err != nil {
		t.Fatalf("SelfUpdate: %v", err)
	}

	got, err := os.ReadFile(current)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("替换后内容 = %q, want %q", got, payload)
	}
	if _, err := os.Stat(current + ".old"); !os.IsNotExist(err) {
		t.Fatal("成功更新后应清理 .old 备份")
	}
}

// TestUpdateCache 验证缓存读写与 done 标志。
func TestUpdateCache(t *testing.T) {
	var c UpdateCache
	if _, done := c.Get(); done {
		t.Fatal("初始状态不应 done")
	}
	c.Set(&UpdateInfo{LatestVersion: "v9.9.9"})
	info, done := c.Get()
	if !done || info == nil || info.LatestVersion != "v9.9.9" {
		t.Fatalf("缓存读写异常: info=%v done=%v", info, done)
	}
}
