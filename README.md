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

## Kaynak sunucu kurulumu (DirectAdmin)

`sftp-backup-ui` yalnızca **hedef** (backup) sunucu tarafını yönetir. Kaynak
tarafında bir DirectAdmin sunucusunun buraya SFTP+SSH-key ile yedek göndermesini
istiyorsanız — DirectAdmin'in native Admin Backup arayüzü SSH key desteklemediği
için — `contrib/directadmin-sftp-backup-setup.sh` bunu otomatikleştirir: custom
bir upload hook'u (`/usr/local/directadmin/scripts/custom/ftp_upload.php`)
kurar, `backup.conf` ve `backup_crons.list`'i günceller.

**Önemli: bu script'i kendi bilgisayarınızda değil, DirectAdmin sunucusunun
kendisinde (SSH ile bağlanıp, root olarak) çalıştırmanız gerekir.**

```sh
scp contrib/directadmin-sftp-backup-setup.sh root@<directadmin-sunucusu>:/root/
ssh root@<directadmin-sunucusu>
./directadmin-sftp-backup-setup.sh --hostname server.example.com --dest-host backup.ornek-sunucunuz.com
```

`--dest-host`/`--dest-port`, sftp-backup-ui'nin çalıştığı **hedef** backup
sunucusunu ve onun SFTP portunu (varsayılan `22`) belirtir — DirectAdmin
sunucusuna bağlanmak için kullandığınız SSH portuyla karıştırmayın, script
bunu hiç bilmesi gerekmez (zaten o sunucunun içinde çalışıyor olacak).

Script bir SSH anahtar çifti üretir (varsa dokunmaz) ve public key'i ekrana basar —
bunu panelde ilgili kullanıcının **"SSH Key ekle/değiştir"** akışına yapıştırın
(veya kullanıcıyı henüz oluşturmadıysanız önce "Yeni Kullanıcı" ile oluşturun).
Ardından bağlantıyı doğrulayın (yine DirectAdmin sunucusunun içinden):

```sh
./directadmin-sftp-backup-setup.sh --test-only --hostname server.example.com --dest-host backup.ornek-sunucunuz.com
```

Script idempotent'tir — aynı parametrelerle tekrar çalıştırmak güvenlidir (anahtar
tekrar üretilmez, hook/cron girişi güncellenir). Tüm seçenekler için
`--help` kullanın.

## rsync ile yedekleme (rsyncd)

SSH-key + SFTP chroot'un yanında, isteyen kaynaklar için **rsync daemon**
üzerinden de yedek alınabilir. Bu, mevcut chroot kullanıcılarına hiç
dokunmayan, tamamen ayrı bir mekanizma (SSH key değil, kullanıcı adı +
parola ile kimlik doğrulama — `secrets file` düz metin sır tutar, bu yüzden
SSH+key kadar güçlü değildir; ek bir seçenek olarak düşünün, yerine değil).

**Tek seferlik sunucu hazırlığı** (`make deploy` bunları otomatik yapmaz,
sistemin paylaşılan config dosyalarına dokunmadan bırakır):

```sh
# 1) rsyncd.service dosyası make deploy ile zaten kopyalandı, sadece etkinleştirin:
mkdir -p /etc/rsyncd.d

# 2) /etc/rsyncd.conf'a temel ayarları + per-user modüllerin include'unu ekleyin:
cat >> /etc/rsyncd.conf <<'EOF'

use chroot = yes
max connections = 10
pid file = /var/run/rsyncd.pid
log file = /var/log/rsyncd.log
timeout = 900

&include /etc/rsyncd.d
EOF

# 3) firewall'da 873/tcp'yi açın:
firewall-cmd --permanent --add-port=873/tcp && firewall-cmd --reload

# 4) servisi başlatın:
systemctl daemon-reload
systemctl enable --now rsyncd
```

Bundan sonra panelde bir kullanıcının satırında **"Rsync Etkinleştir"**
butonuna basmak yeterli — modül + rastgele parola otomatik üretilir ve
**bir kereliğine** ekranda gösterilir (kopyalayıp kaynak sunucuya aktarın,
panelde tekrar görüntülenemez).

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
| `RSYNCD_DIR` | `/etc/rsyncd.d` | Yönetilen rsync modül/secret dosyalarının yeri |
| `RSYNCD_SERVICE_NAME` | `rsyncd` | rsync daemon'ı için systemd servis adı |

## Lisans

MIT — bkz. [LICENSE](LICENSE).
