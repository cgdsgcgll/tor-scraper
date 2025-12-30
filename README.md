# Tor-Scraper

Tor ağı (.onion uzantılı web siteleri) üzerinde çalışan, Go (Golang) ile geliştirilmiş
otomatik bir web scraper uygulamasıdır. Uygulama, Tor Browser tarafından sağlanan
SOCKS5 proxy üzerinden anonim bağlantı kurarak hedef sitelerden HTML içeriklerini
toplamakta ve erişilebilir sitelerin ekran görüntülerini almaktadır.

Bu proje, eğitim ve akademik amaçlarla geliştirilmiştir.

---

## 🚀 Özellikler

- Tor ağı üzerinden (.onion) web sitelerine erişim
- SOCKS5 proxy desteği (127.0.0.1:9150)
- Otomatik HTML içerik kaydı
- Headless tarayıcı (chromedp) ile ekran görüntüsü alma
- YAML tabanlı hedef site yönetimi
- Aktif / pasif site raporlaması
- Loglama (scan_report.log)

---

## 🛠️ Kullanılan Teknolojiler

- **Go (Golang)**
- **Tor Browser / Tor Service**
- **SOCKS5 Proxy**
- **net/http**
- **chromedp**
- **YAML**

---

## 📂 Proje Yapısı

tor-scraper/
├── main.go
├── go.mod
├── go.sum
├── target.yaml
├── scan_report.log
├── output/
│ ├── html/
│ └── screenshots/

---

## ⚙️ Kurulum ve Çalıştırma

### 1️⃣ Gerekli Yazılımlar

- Go
- Tor Browser (çalışır durumda olmalı)

### 2️⃣ Tor SOCKS5 Proxy Kontrolü

```powershell
Test-NetConnection 127.0.0.1 -Port 9150

### 3️⃣ Uygulamayı Çalıştırma
go run . -targets target.yaml -socks 127.0.0.1:9150 -shots=true

### 4️⃣ Derlenmiş Binary ile Çalıştırma
.\tor-scraper.exe -targets target.yaml -socks 127.0.0.1:9150 -shots=true
```
