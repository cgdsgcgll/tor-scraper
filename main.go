package main

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"golang.org/x/net/proxy"
)

/* ----------------- HELPERS ----------------- */

var nonSafe = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func ensureDir(p string) error { return os.MkdirAll(p, 0755) }

func safeNameFromURL(u string) string {
	base := nonSafe.ReplaceAllString(u, "_")
	if len(base) > 60 {
		base = base[:60]
	}
	h := sha1.Sum([]byte(u))
	return base + "_" + hex.EncodeToString(h[:6])
}

/* ----------------- LOG WRITER ----------------- */

type logger struct {
	f  *os.File
	bw *bufio.Writer
}

func newLogger(path string) (*logger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644) // her çalıştırmada temiz başla
	if err != nil {
		return nil, err
	}
	return &logger{f: f, bw: bufio.NewWriterSize(f, 64*1024)}, nil
}

func (l *logger) Close() {
	_ = l.bw.Flush()
	_ = l.f.Close()
}

func (l *logger) Println(line string) {
	// Hem dosyaya yaz hem de flush et ki “boş dosya” kalmasın
	_, _ = l.bw.WriteString(line + "\n")
	_ = l.bw.Flush()
}

/* ----------------- TARGETS ----------------- */

func readTargets(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var targets []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "-")
		line = strings.TrimSpace(line)
		if line != "" {
			targets = append(targets, line)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("target list is empty")
	}
	return targets, nil
}

/* ----------------- TOR HTTP CLIENT ----------------- */

func newTorHTTPClient(socksAddr string) (*http.Client, error) {
	dialer, err := proxy.SOCKS5("tcp", socksAddr, nil, proxy.Direct)
	if err != nil {
		return nil, err
	}

	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		// x/net/proxy dialer context bilmeyebilir, ama timeout'u http.Client üstünden yönetiyoruz
		return dialer.Dial(network, addr)
	}

	tr := &http.Transport{
		DialContext:           dialContext,
		DisableKeepAlives:     true,
		ResponseHeaderTimeout: 25 * time.Second,
		IdleConnTimeout:       10 * time.Second,
	}

	return &http.Client{
		Transport: tr,
		Timeout:   45 * time.Second,
	}, nil
}

func fetchHTML(client *http.Client, url string) ([]byte, int, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", "Tor-Scraper/1.0 (Educational)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	// 200-399 arası OK sayalım
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return b, resp.StatusCode, fmt.Errorf("bad status: %s", resp.Status)
	}
	return b, resp.StatusCode, nil
}

/* ----------------- SCREENSHOT (CHROMEDP) ----------------- */

type shooter struct {
	allocCtx context.Context
	cancel   context.CancelFunc
}

func newShooter(socksAddr string) (*shooter, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
    chromedp.Flag("headless", true),
    chromedp.Flag("disable-gpu", true),
    chromedp.Flag("no-sandbox", true),
    chromedp.Flag("disable-dev-shm-usage", true),
    chromedp.Flag("ignore-certificate-errors", true),

    // SADECE 1 KERE:
    chromedp.Flag("proxy-server", "socks5://"+socksAddr),

    chromedp.WindowSize(1366, 768),
)


	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	return &shooter{allocCtx: allocCtx, cancel: cancel}, nil
}

func (s *shooter) Close() { s.cancel() }

func (s *shooter) Screenshot(url, outPath string) error {
	ctx, cancel := chromedp.NewContext(s.allocCtx)
	defer cancel()

	// Onion yavaş olabilir
	ctx, cancel2 := context.WithTimeout(ctx, 90*time.Second)
	defer cancel2()

	var png []byte
	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(2*time.Second),
		chromedp.FullScreenshot(&png, 90),
	)
	if err != nil {
		return err
	}
	if len(png) == 0 {
		return fmt.Errorf("empty png buffer")
	}
	return os.WriteFile(outPath, png, 0644)
}

/* ----------------- MAIN ----------------- */

func main() {
	targetFile := flag.String("targets", "target.yaml", "target file")
	socks := flag.String("socks", "127.0.0.1:9150", "Tor SOCKS5 addr (Tor Browser: 127.0.0.1:9150)")
	shots := flag.Bool("shots", true, "take screenshots for successful targets")
	flag.Parse()

	_ = ensureDir(filepath.Join("output", "html"))
	_ = ensureDir(filepath.Join("output", "screenshots"))

	logg, err := newLogger("scan_report.log")
	if err != nil {
		fmt.Println("[FATAL] cannot open scan_report.log:", err)
		return
	}
	defer logg.Close()

	logg.Println("=== Scan started at " + time.Now().Format(time.RFC3339) + " ===")

	targets, err := readTargets(*targetFile)
	if err != nil {
		fmt.Println("[FATAL]", err)
		logg.Println("[FATAL] " + err.Error())
		return
	}

	client, err := newTorHTTPClient(*socks)
	if err != nil {
		fmt.Println("[FATAL] tor client:", err)
		logg.Println("[FATAL] tor client: " + err.Error())
		return
	}

	var sh *shooter
	if *shots {
		sh, err = newShooter(*socks)
		if err != nil {
			fmt.Println("[WARN] chromedp init failed:", err)
			logg.Println("[WARN] chromedp init failed: " + err.Error())
			sh = nil
		} else {
			defer sh.Close()
			logg.Println("[INFO] screenshots enabled -> output/screenshots/")
		}
	}

	okCount, errCount := 0, 0

	for _, url := range targets {
		fmt.Println("[INFO] Scanning:", url)
		logg.Println("[INFO] Scanning: " + url)

		body, status, ferr := fetchHTML(client, url)
		if ferr != nil {
			line := fmt.Sprintf("[ERR] %s status=%d err=%v", url, status, ferr)
			fmt.Println(line)
			logg.Println(line)
			errCount++
			continue
		}

		if len(body) == 0 {
			w := fmt.Sprintf("[WARN] %s status=%d -> empty HTML body", url, status)
			fmt.Println(w)
			logg.Println(w)
		}

		htmlOut := filepath.Join("output", "html", safeNameFromURL(url)+".html")
		if werr := os.WriteFile(htmlOut, body, 0644); werr != nil {
			line := fmt.Sprintf("[ERR] %s write_html_fail=%v", url, werr)
			fmt.Println(line)
			logg.Println(line)
			errCount++
			continue
		}

		// Screenshot
		if sh != nil {
			pngOut := filepath.Join("output", "screenshots", safeNameFromURL(url)+".png")
			if serr := sh.Screenshot(url, pngOut); serr != nil {
				line := fmt.Sprintf("[OK]  %s saved_html=%s screenshot_fail=%v", url, htmlOut, serr)
				fmt.Println(line)
				logg.Println(line)
			} else {
				line := fmt.Sprintf("[OK]  %s saved_html=%s saved_png=%s", url, htmlOut, pngOut)
				fmt.Println(line)
				logg.Println(line)
			}
		} else {
			line := fmt.Sprintf("[OK]  %s saved_html=%s", url, htmlOut)
			fmt.Println(line)
			logg.Println(line)
		}

		okCount++
	}

	logg.Println(fmt.Sprintf("=== Summary OK=%d ERR=%d ===", okCount, errCount))
	logg.Println("=== Scan finished at " + time.Now().Format(time.RFC3339) + " ===")
	fmt.Println("[DONE]")
}
