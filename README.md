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

## Chroot + gerçek shell: SFTP'nin yanında rsync/scp de otomatik çalışır

"SFTP Etkinleştir" (veya yeni kullanıcı oluşturma), kullanıcıyı sadece
SFTP'ye değil, kendi chroot'u içinde çalışan gerçek bir shell'e de kavuşturur
— ayrı bir "rsync'i aç" adımı yoktur, bu SFTP kurulumunun kendisinin bir
parçasıdır. Pratikte bunun anlamı: aynı SSH key ile hem SFTP hem `rsync -e
ssh` hem `scp` çalışır — cPanel WHM'in native **"Rsync"** backup hedef tipi
dahil (o, SSH üzerinden `rsync --server` çalıştırır; salt SFTP-forced bir
hesapta bu "bad password or master process exited unexpectedly" gibi
yanıltıcı bir hatayla başarısız olurdu).

Mekanizma: sshd'nin `Match User` bloğunda `ForceCommand` hiç yazılmaz.
SFTP subsystem isteği zaten global `Subsystem sftp` yapılandırmasından
geçtiği için etkilenmez; `ForceCommand` olmayınca doğrudan komut çalıştırma
(`rsync -e ssh`) ve interactive login da artık kullanıcının gerçek shell'ine
(`/bin/bash`) ulaşır. Bu shell'in chroot içinde çalışabilmesi için gereken
`bash`, `rsync`, `sftp-server` binary'leri, birkaç temel coreutils komutu
(`mkdir`, `mv`, `rm`, `cat`, `chmod`, `stat`, `df`, `test`, `ls` —
`internal/sysops/chrootenv.go`'daki `coreutilsPaths`) ve bunların güncel
shared library bağımlılıkları (`ldd` ile her etkinleştirmede taze
hesaplanır), `/dev/null`, ve sadece o kullanıcının kendi kaydını içeren
minimal `/etc/passwd`+`/etc/group` — hepsi chroot'a **salt-okunur
bind-mount** edilir (host'taki gerçek dosyalara bağlı, kopya değil) ve
`/etc/fstab`'a kalıcı olarak yazılır.

Bu coreutils seti baştan değil, gerçek doğrulama denemelerinde ortaya çıkan
"child exited with code 127" hatalarından sonra eklendi: cPanel WHM'in
kendi Rsync hedef doğrulaması, test dosyasını rsync ile yükleyip ardından
düz bir `mv` komutuyla yeniden adlandırıyor; rsync-over-ssh transport'ları
genel olarak hedef dizini `mkdir -p` gibi düz kabuk komutlarıyla da
oluşturabiliyor. Yani bu, önceden tahmin edilip eklenmiş değil, gerçek
istemci davranışına göre genişletilmiş bir liste.

**Bilinçli sınır:** chroot'a `vim`, `ps`, `grep`, `tar` gibi daha geniş
interaktif/idari araçlar eklenmez — sadece rsync/sftp/scp'nin, onu
doğrulayan/tetikleyen otomasyon araçlarının (cPanel, DirectAdmin vb.) ve
temel dosya listeleme/yönetiminin ihtiyaç duyabileceği komutlar var. Bir
kullanıcı interactive SSH login yaparsa yukarıdaki listenin dışında
çalıştırabileceği harici bir komut yoktur; host filesystem'ine ya da diğer
kullanıcılara hiçbir erişimi olmaz (chroot hâlâ geçerli).

**Bakım notu:** bind-mount'lar host'taki gerçek dosyaya bağlı olduğu için
çoğu güncelleme (paket içeriği değişse bile) otomatik yansır; ama bir
`dnf update` dosyayı olduğu yerde değil de yeniden oluşturarak değiştirirse
mount "bayatlayabilir" — böyle bir durumda ilgili kullanıcıda "SFTP +
Rsync Etkinleştir"e tekrar basmak (idempotent, mount'ları tazeler)
yeterlidir.

## NFS ile yedekleme (disk alanı yetersiz kaynak sunucular için)

Bazı kaynak sunucuların diski o kadar dolu ki, yedekleme yazılımı önce
yerelde bir arşiv oluşturup sonra SFTP/rsync ile göndermek için yeterli boş
alan bulamıyor — bu yüzden hiç yedek alamıyorlar. Bunun çözümü: bu
kullanıcının `upload/` dizinini **NFSv4** ile doğrudan kaynak sunucuya
mount etmek; yedekleme yazılımı hiç yerel disk kullanmadan doğrudan ağ
üzerinden buraya yazar.

Bu, SFTP/SSH-key modelinden **tamamen ayrı, farklı bir güvenlik modeline**
sahip bir erişim yöntemidir — SSH-key yerine **IP tabanlı** kimlik
doğrulama kullanır, kriptografik olarak SSH-key kadar güçlü değildir.
Panelde bir kullanıcı için "NFS Etkinleştir"e bastığınızda:

- Kaynak sunucunun IP'sini (birden fazla olabilir) girersiniz — sadece o
  IP'ler bu export'u mount edebilir (`/etc/exports.d/<kullanıcı>.exports`,
  `sshd_config.d` ile aynı include-dosyası mantığı, ana `/etc/exports`'a
  hiç dokunulmaz).
- Export edilen dizine yazan **herkes** (kaynak sunucuda root dahil, hangi
  uid ile yazarsa yazsın) `all_squash` ile bu kullanıcının kendi sabit
  uid/gid'ine eşlenir — kaynak sunucu tamamen ele geçirilse bile saldırgan
  sadece bu kullanıcının kendi kota-sınırlı dizinine yazabilir, başka
  hiçbir şeye erişemez.
- Aynı `upload/` dizini olduğu için mevcut XFS project kotası olduğu gibi
  geçerli olmaya devam eder (kota dizin bazlı çalışır, protokolden
  bağımsız) ve panel yine aynı dosyaları "yedekleme" olarak tanır. Gerçek
  bir Linux istemciden `dd` ile yazılan veri, doğrulama sırasında
  `xfs_quota report`'a anında yansıdı.
- Export `insecure` ile açılır (istemcinin ayrıcalıklı `<1024` porttan
  bağlanma zorunluluğu yoktur) — varsayılan "secure" davranış, kaynağın
  NAT/load-balancer arkasında olduğu (orijinal kaynak portu korunmayan) her
  durumda mount'u sessizce "Operation not permitted" ile reddediyor; bu,
  gerçek bir mount denemesinde doğrulandı.

**Tek seferlik sunucu hazırlığı** (`make deploy` bunu otomatik yapmaz):

```sh
dnf install -y nfs-utils

# Sadece NFSv4 - v2/v3 hiç açılmaz, firewall'da tek port yeterli olur:
sed -i '/^\[nfsd\]/,/^\[/{s/^#\?vers3=.*/vers3=n/}' /etc/nfs.conf
grep -q '^\[nfsd\]' /etc/nfs.conf || printf '\n[nfsd]\nvers3=n\n' >> /etc/nfs.conf

systemctl enable --now nfs-server
firewall-cmd --permanent --add-port=2049/tcp && firewall-cmd --reload
```

**Kaynak sunucu tarafında** (panelin gösterdiği hazır komut — `/etc/fstab`'a
kalıcı bir satır ekleyip hemen mount eder):

```sh
mkdir -p /backup && echo "<backup-sunucusu>:<kullanıcının upload dizini> /backup nfs4 rw,_netdev,noatime 0 0" >> /etc/fstab && systemctl daemon-reload && mount /backup
```

Mount, kaynak sunucuda **root** gerektirir — SSH-key modelinden farklı
olarak burada root yetkisi şart; bu, hedef (backup) sunucusundaki bu araç
tarafından değil, kaynak sunucuda elle yapılmalıdır. Mount tamamlandıktan
sonra DirectAdmin/cPanel'in yedekleme hedefi bu mount noktasına
("Local"/"Yerel" hedef tipi) gösterilir.

"NFS Kapat", SFTP/SSH-key erişimine hiç dokunmadan sadece export'u kaldırır.

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
| `NFS_EXPORTS_DIR` | `/etc/exports.d` | Yönetilen per-kullanıcı NFS export dosyalarının yeri |
| `NFS_SERVICE_NAME` | `nfs-server` | `exportfs`'in dayandığı systemd servis adı (aktiflik kontrolü için) |

## Lisans

MIT — bkz. [LICENSE](LICENSE).
