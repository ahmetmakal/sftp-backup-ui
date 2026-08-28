# SFTP Backup UI

Bir backup sunucusunda, SFTP ile yedek alacak kaynak sunucular için sistem kullanıcısı
(chroot'lu SFTP erişimi), XFS project kotası ve SSH key kurulumunu tek bir web
arayüzünden yönetmeyi sağlayan küçük bir Go/Gin uygulaması.

Bu araç, aşağıdaki elle yapılan çok adımlı işlemi otomatikleştirir ve doğrular:

```
useradd -d /backup1/<user> -s /usr/sbin/nologin <user>
mkdir -p /backup1/<user>/upload
chown root:root /backup1/<user> && chmod 755 /backup1/<user>
chown <user>:<user> /backup1/<user>/upload
# sshd Match User <user> ... ChrootDirectory ... ForceCommand internal-sftp
# xfs_quota -x -c 'project -s <user>' ... && 'limit -p bsoft=... bhard=...' ...
# .ssh/authorized_keys yazımı + doğru sahiplik/izin
```

## Özellikler

- **Veritabanı yok.** Kullanıcı listesi, kotalar ve SFTP durumu her sayfa
  yüklemesinde doğrudan sistemden (`/etc/passwd`, `xfs_quota report`, sshd config,
  `/proc/mounts`) canlı okunur — UI hiçbir zaman sistemin gerçek durumundan
  sapmış bir kayıt tutmaz.
- Backup mount'ları (`/backup1`, `/backup2`, ...) sabit kodlanmaz; XFS +
  `prjquota` desteğine sahip `/backupN` desenindeki mount'lar otomatik keşfedilir.
- Kullanıcı oluşturma sihirbazı: useradd → dizin/izin kurulumu → sshd chroot
  drop-in yapılandırması → XFS proje kotası → (opsiyonel) SSH key, hepsi tek
  formdan, hatalı adımda best-effort geri alma ile.
- Kota güncelleme, SSH key ekleme/değiştirme, kullanıcı silme (veri dahil/hariç).
- Mevcut/manuel kurulmuş kullanıcıları da tanır (elle eklenmiş sshd `Match User`
  bloklarını okuyabilir) ve tek tıkla yönetime alabilir.
- Her kullanıcının `upload/` dizinine bakarak yedeğin cPanel/DirectAdmin/özel
  formatta olup olmadığını, son yedekleme zamanını ve hesap sayısını tahmin eder.
- Tablo üzerinde arama ve sıralama.
- Tailwind CSS ile açık/koyu tema, tek admin şifresiyle basic-auth.

## Gereksinimler

- Linux backup sunucusu, **root** yetkisiyle çalıştırılır (useradd, xfs_quota,
  sshd reload gibi root gerektiren komutlar çalıştırır).
- `xfs_quota` kurulu ve backup mount'ları `prjquota`/`pquota` seçeneğiyle mount edilmiş.
- OpenSSH sunucusunda `Include /etc/ssh/sshd_config.d/*.conf` (modern dağıtımlarda varsayılan).
- Go 1.25+ ve Node.js/npm (sadece Tailwind CSS derlemek için, runtime bağımlılığı değil).

## Kurulum ve Deploy

```sh
git clone https://github.com/ahmetmakal/sftp-backup-ui.git
cd sftp-backup-ui

# Uzak backup sunucusunu belirtin (varsayılanlar Makefile'da placeholder):
make deploy REMOTE_HOST=backup.ornek-sunucunuz.com REMOTE_USER=root
```

`make deploy`: Tailwind CSS'i derler, Go binary'sini `linux/amd64` için derler,
`templates/`, `static/`, binary'yi ve systemd servis dosyasını uzak sunucuya
gönderir, servisi kurup başlatır. İlk deploy'da `/etc/sftp-backup-ui/env`
dosyası `deploy/env.example`'dan oluşturulur (üzerine yazılmaz) — mutlaka
`ADMIN_PASSWORD`'u değiştirin:

```sh
make ssh   # sunucuya bağlan
vi /etc/sftp-backup-ui/env
make restart
```

Diğer komutlar: `make status`, `make logs`, `make build` (sadece derle).

## Yerelde çalıştırma

Uygulama Linux'a özgü sistem çağrıları (`/proc/mounts`, `useradd`, `xfs_quota`)
kullandığı için gerçek işlevselliği yalnızca Linux'ta test edilebilir. Yine de
şablon/route/derleme kontrolü için:

```sh
make css   # static/app.css üretir
GOOS=linux GOARCH=amd64 go build ./...
```

## Konfigürasyon (env var'lar)

| Değişken | Varsayılan | Açıklama |
|---|---|---|
| `LISTEN_ADDR` | `:8080` | HTTP dinleme adresi |
| `ADMIN_USER` | `admin` | Basic-auth kullanıcı adı |
| `ADMIN_PASSWORD` | — (zorunlu) | Basic-auth şifresi |
| `BACKUP_MOUNT_REGEX` | `^/backup[0-9]+$` | Mount keşfi için desen |
| `SSHD_CONFIG_DIR` | `/etc/ssh/sshd_config.d` | Yönetilen drop-in dosyalarının yeri |
| `SSHD_MAIN_CONFIG` | `/etc/ssh/sshd_config` | Manuel `Match User` bloklarını tespit için |
| `SSHD_SERVICE_NAME` | `sshd` | `systemctl reload` için servis adı |
| `PROJECT_ID_BASE` | `100` | XFS proje ID ataması için taban değer |

## Lisans

MIT — bkz. [LICENSE](LICENSE).
