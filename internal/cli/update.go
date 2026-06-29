package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const ghRepo = "abdorizak/sm2"

func newUpdateCmd() *cobra.Command {
	var (
		checkOnly bool
		pinned    string
	)
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update sm2 to the latest release",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(checkOnly, pinned)
		},
	}
	cmd.Flags().BoolVar(&checkOnly, "check", false, "only check for a newer version; don't install")
	cmd.Flags().StringVar(&pinned, "version", "", "install a specific version (e.g. v0.1.0-dev.3)")
	return cmd
}

func runUpdate(checkOnly bool, pinned string) error {
	client := &http.Client{Timeout: 30 * time.Second}

	latest := pinned
	if latest == "" {
		var err error
		latest, err = latestRelease(client)
		if err != nil {
			return err
		}
	}

	fmt.Printf("current: %s\nlatest:  %s\n", version, latest)
	if latest == version {
		fmt.Println("already up to date ✓")
		return nil
	}
	if checkOnly {
		fmt.Printf("a newer version is available — run `%s update` to install it\n", invokedName())
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate current binary: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	if err := downloadAndReplace(client, latest, exe); err != nil {
		return err
	}
	fmt.Printf("updated %s → %s\n", version, latest)
	fmt.Printf("run `%s kill` to restart the agent on the new version (apps auto-resurrect)\n", invokedName())
	return nil
}

// latestRelease returns the newest release tag (including pre-releases).
func latestRelease(client *http.Client) (string, error) {
	body, err := httpGet(client, "https://api.github.com/repos/"+ghRepo+"/releases")
	if err != nil {
		return "", fmt.Errorf("check latest release: %w", err)
	}
	var rels []struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &rels); err != nil {
		return "", fmt.Errorf("parse releases: %w", err)
	}
	if len(rels) == 0 {
		return "", fmt.Errorf("no releases found")
	}
	return rels[0].TagName, nil
}

func downloadAndReplace(client *http.Client, tag, exe string) error {
	asset := fmt.Sprintf("sm2_%s_%s_%s.tar.gz", tag, runtime.GOOS, runtime.GOARCH)
	base := fmt.Sprintf("https://github.com/%s/releases/download/%s", ghRepo, tag)

	fmt.Printf("downloading %s …\n", asset)
	archive, err := httpGet(client, base+"/"+asset)
	if err != nil {
		return fmt.Errorf("download %s: %w", asset, err)
	}

	// Verify the checksum when SHA256SUMS is published (best-effort).
	if sums, err := httpGet(client, base+"/SHA256SUMS"); err == nil {
		if err := verifyChecksum(sums, asset, archive); err != nil {
			return err
		}
	}

	bin, err := extractFile(archive, "sm2")
	if err != nil {
		return err
	}

	// Write next to the current binary and atomically replace it.
	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, ".sm2-update-*")
	if err != nil {
		return fmt.Errorf("cannot write to %s — try `sudo %s update`: %w", dir, invokedName(), err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(bin); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmpName, exe); err != nil {
		return fmt.Errorf("replace %s — try `sudo %s update`: %w", exe, invokedName(), err)
	}
	return nil
}

func httpGet(client *http.Client, url string) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// verifyChecksum confirms data matches the SHA256SUMS entry for asset.
func verifyChecksum(sums []byte, asset string, data []byte) error {
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[1] == asset {
			want = f[0]
			break
		}
	}
	if want == "" {
		return nil // no entry for this asset; skip
	}
	got := sha256.Sum256(data)
	if hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("checksum mismatch for %s", asset)
	}
	return nil
}

// extractFile pulls a single named file out of a .tar.gz archive.
func extractFile(targz []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(targz))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("%q not found in archive", name)
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(hdr.Name) == name {
			return io.ReadAll(tr)
		}
	}
}
