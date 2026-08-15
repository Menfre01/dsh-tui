package tui

// update.go — GitHub Release 版本检查与自更新(移植自 waveloom
// pkg/environment 的 self_update.go + update_check.go,适配 dsh-tui)。

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// UpdateInfo 包含版本更新检查的结果。
type UpdateInfo struct {
	CurrentVersion  string // 当前编译版本号(如 "v0.1.0")
	LatestVersion   string // GitHub 最新 release tag(如 "v0.2.0")
	UpdateAvailable bool   // 是否有新版本可用
	URL             string // release 页面 URL
}

// CheckForUpdate 获取 GitHub 最新 release tag 并与当前版本比较。
// 通过访问 releases/latest 重定向地址提取 tag,无需 API 认证,不受限流。
// 网络错误静默忽略,返回 (nil, nil) 表示跳过本次检查。
func CheckForUpdate(ctx context.Context, currentVersion string) (*UpdateInfo, error) {
	// 开发版本不检查
	if currentVersion == "" || currentVersion == "dev" {
		return nil, nil
	}
	return checkLatestRelease(ctx, "https://github.com/Menfre01/dsh-tui/releases/latest", currentVersion)
}

// checkLatestRelease 访问 releases/latest 页面,从重定向目标 URL 中提取 tag。
// GitHub releases/latest 返回 302 到 /releases/tag/<version>,无需 API 认证、不受限流。
// URL 参数化便于 httptest mock。
func checkLatestRelease(ctx context.Context, url, currentVersion string) (*UpdateInfo, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // 不跟随重定向,只需要 Location header
		},
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("check update: %w", err)
	}
	req.Header.Set("User-Agent", "dsh-tui/"+currentVersion)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("check update: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// releases/latest 返回 302,Location header 包含 /releases/tag/<version>
	if resp.StatusCode != http.StatusFound {
		return nil, fmt.Errorf("check update: unexpected status %d", resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	tag := location
	if idx := strings.LastIndex(location, "/"); idx >= 0 {
		tag = location[idx+1:]
	}

	return &UpdateInfo{
		CurrentVersion:  currentVersion,
		LatestVersion:   tag,
		UpdateAvailable: isUpdateAvailable(currentVersion, tag),
		URL:             location,
	}, nil
}

// isUpdateAvailable 语义化比较当前版本与最新 tag。
// git describe 注入的版本可能带后缀(如 v0.0.1-7-gbd571cc-dirty),
// 只比较 vX.Y.Z 主版本段,避免本地 dirty 构建误报更新。
func isUpdateAvailable(currentVersion, latestTag string) bool {
	cur := semverSegment(currentVersion)
	lat := semverSegment(latestTag)
	if cur == "" || lat == "" {
		return false
	}
	return compareSemver(lat, cur) > 0
}

// semverSegment 提取 "vX.Y.Z" 主版本段(去掉 v 前缀与 - 后缀)。
// 非数字开头(dev、"" 等)返回空,视为不可比较。
func semverSegment(v string) string {
	v = strings.TrimPrefix(v, "v")
	if idx := strings.IndexByte(v, '-'); idx >= 0 {
		v = v[:idx]
	}
	if v == "" {
		return ""
	}
	if v[0] < '0' || v[0] > '9' {
		return ""
	}
	return v
}

// compareSemver 比较 X.Y.Z 三段数字;left > right 返回正数,相等返回 0。
func compareSemver(left, right string) int {
	lp := strings.Split(left, ".")
	rp := strings.Split(right, ".")
	for i := 0; i < 3; i++ {
		lv, rv := 0, 0
		if i < len(lp) {
			lv = atoiOrZero(lp[i])
		}
		if i < len(rp) {
			rv = atoiOrZero(rp[i])
		}
		if lv != rv {
			return lv - rv
		}
	}
	return 0
}

// atoiOrZero 解析数字,失败返回 0。
func atoiOrZero(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// CheckForUpdateAsync 在后台 goroutine 执行更新检查,结果通过 channel 返回。
// 2s 超时保护,失败时静默跳过。
func CheckForUpdateAsync(currentVersion string) <-chan *UpdateInfo {
	ch := make(chan *UpdateInfo, 1)
	go func() {
		defer close(ch)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		info, err := CheckForUpdate(ctx, currentVersion)
		if err != nil || info == nil {
			return
		}
		ch <- info
	}()
	return ch
}

// ---------------------------------------------------------------------------
// 线程安全缓存(防止 render 循环中重复检查)
// ---------------------------------------------------------------------------

// UpdateCache 线程安全缓存单次更新检查结果。
type UpdateCache struct {
	mu   sync.RWMutex
	info *UpdateInfo
	done bool
}

// Set 写入检查结果。
func (c *UpdateCache) Set(info *UpdateInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.info = info
	c.done = true
}

// Get 读取检查结果。done 表示检查已完成。
func (c *UpdateCache) Get() (*UpdateInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.info, c.done
}

// ---------------------------------------------------------------------------
// 自更新流程
// ---------------------------------------------------------------------------

// SelfUpdatePhase 表示自更新流程的阶段。
type SelfUpdatePhase string

const (
	PhaseDownload SelfUpdatePhase = "download"
	PhaseExtract  SelfUpdatePhase = "extract"
	PhaseInstall  SelfUpdatePhase = "install"
	PhaseDone     SelfUpdatePhase = "done"
)

// SelfUpdateProgress 是自更新进度回调。
type SelfUpdateProgress func(phase SelfUpdatePhase, pct int, detail string)

// BuildDownloadURL 返回当前平台对应的 GitHub Release 下载地址。
// Windows 使用 .zip,其他平台使用 .tar.gz。
func BuildDownloadURL() string {
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf(
		"https://github.com/Menfre01/dsh-tui/releases/latest/download/dsh-tui_%s_%s.%s",
		runtime.GOOS, runtime.GOARCH, ext,
	)
}

// SelfUpdate 下载 release 包到临时目录,解压并替换 currentPath 指向的二进制文件。
// 成功时新二进制已替换 currentPath;失败时尝试回滚(从 .old 备份恢复)。
// progress 可为 nil,此时不报告进度。
func SelfUpdate(ctx context.Context, currentPath, downloadURL string, progress SelfUpdateProgress) error {
	report := func(phase SelfUpdatePhase, pct int, detail string) {
		if progress != nil {
			progress(phase, pct, detail)
		}
	}

	// Phase 1: 下载
	report(PhaseDownload, 0, fmt.Sprintf("Downloading %s ...", filepath.Base(downloadURL)))

	tmpDir, err := os.MkdirTemp("", "dsh-tui-update-")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	isZip := strings.HasSuffix(downloadURL, ".zip")
	archiveExt := ".tar.gz"
	if isZip {
		archiveExt = ".zip"
	}
	archivePath := filepath.Join(tmpDir, "dsh-tui"+archiveExt)
	if err := downloadWithProgress(ctx, downloadURL, archivePath, func(downloaded, total int64, pct int) {
		mbDown := float64(downloaded) / (1024 * 1024)
		mbTotal := float64(total) / (1024 * 1024)
		report(PhaseDownload, pct, fmt.Sprintf("  %.1f MB / %.1f MB (%d%%)", mbDown, mbTotal, pct))
	}); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Phase 2: 解压
	report(PhaseExtract, 100, "Extracting ...")

	var newBinary string
	if isZip {
		newBinary, err = extractBinaryZip(archivePath, tmpDir)
	} else {
		newBinary, err = extractBinaryTarGz(archivePath, tmpDir)
	}
	if err != nil {
		return fmt.Errorf("extract failed: %w", err)
	}

	// Phase 3: 安装
	report(PhaseInstall, 100, fmt.Sprintf("Installing to %s ...", currentPath))

	backupPath := currentPath + ".old"
	if err := os.Rename(currentPath, backupPath); err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}

	if err := copyFile(newBinary, currentPath); err != nil {
		// 回滚
		_ = os.Rename(backupPath, currentPath)
		return fmt.Errorf("install failed: %w", err)
	}

	_ = os.Remove(backupPath)

	// Windows: os.Chmod 仅影响写权限位,设置 0o755 会报错
	if runtime.GOOS != "windows" {
		if err := os.Chmod(currentPath, 0o755); err != nil {
			return fmt.Errorf("chmod failed: %w", err)
		}
	}

	report(PhaseDone, 100, "installed")
	return nil
}

// ---------------------------------------------------------------------------
// 内部实现
// ---------------------------------------------------------------------------

// downloadWithProgress 下载文件,通过回调报告进度。
func downloadWithProgress(ctx context.Context, url, dst string, onProgress func(downloaded, total int64, pct int)) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "dsh-tui")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	total := resp.ContentLength
	pr := &progressReader{
		r:     resp.Body,
		total: total,
		onProgress: func(downloaded int64) {
			pct := 0
			if total > 0 {
				pct = int(downloaded * 100 / total)
			}
			onProgress(downloaded, total, pct)
		},
	}

	_, err = io.Copy(f, pr)
	return err
}

// progressReader 包装 io.Reader,每读取数据后调用一次回调。
type progressReader struct {
	r          io.Reader
	total      int64
	downloaded int64
	onProgress func(downloaded int64)
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.r.Read(p)
	pr.downloaded += int64(n)
	if n > 0 && pr.onProgress != nil {
		pr.onProgress(pr.downloaded)
	}
	return n, err
}

// extractBinaryTarGz 从 tar.gz 中提取名为 "dsh-tui" 的二进制到临时目录。
func extractBinaryTarGz(tarballPath, tmpDir string) (string, error) {
	f, err := os.Open(tarballPath)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	gzReader, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer func() { _ = gzReader.Close() }()

	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if header.Name == "dsh-tui" && header.Typeflag == tar.TypeReg {
			outPath := filepath.Join(tmpDir, "dsh-tui")
			out, err := os.Create(outPath)
			if err != nil {
				return "", err
			}
			// 500MB 上限防解压炸弹(G110);按 header.Size 校验完整性
			if header.Size < 0 || header.Size > 500<<20 {
				_ = out.Close()
				return "", fmt.Errorf("binary size %d exceeds limit", header.Size)
			}
			if _, err := io.Copy(out, io.LimitReader(tarReader, header.Size)); err != nil {
				_ = out.Close()
				return "", err
			}
			_ = out.Close()
			if runtime.GOOS != "windows" {
				if err := os.Chmod(outPath, 0o755); err != nil {
					return "", err
				}
			}
			return outPath, nil
		}
	}
	return "", fmt.Errorf("dsh-tui binary not found in tarball")
}

// extractBinaryZip 从 .zip 中提取 dsh-tui 二进制(Windows 上为 dsh-tui.exe)。
func extractBinaryZip(zipPath, tmpDir string) (string, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", err
	}
	defer func() { _ = r.Close() }()

	binName := "dsh-tui"
	if runtime.GOOS == "windows" {
		binName = "dsh-tui.exe"
	}

	for _, f := range r.File {
		if f.Name == binName {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			defer func() { _ = rc.Close() }()

			outPath := filepath.Join(tmpDir, binName)
			out, err := os.Create(outPath)
			if err != nil {
				return "", err
			}
			defer func() { _ = out.Close() }()

			// 500MB 上限防解压炸弹(G110);按 header.Size 校验完整性
			if f.UncompressedSize64 > 500<<20 {
				return "", fmt.Errorf("binary size %d exceeds limit", f.UncompressedSize64)
			}
			if _, err := io.Copy(out, io.LimitReader(rc, int64(f.UncompressedSize64))); err != nil {
				return "", err
			}
			return outPath, nil
		}
	}
	return "", fmt.Errorf("dsh-tui binary not found in zip")
}

// copyFile 复制文件内容及权限。
func copyFile(src, dst string) error {
	srcF, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = srcF.Close() }()

	dstF, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = dstF.Close() }()

	_, err = io.Copy(dstF, srcF)
	return err
}
