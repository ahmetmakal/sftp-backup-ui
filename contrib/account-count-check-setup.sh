#!/usr/bin/env bash
#
# account-count-check-setup.sh - idempotent setup of a daily account-count
# self-report on a cPanel or DirectAdmin source server, targeting an
# sftp-backup-ui destination user.
#
# sftp-backup-ui has no way to know whether a source server's backups
# actually cover every account it hosts - it only sees what landed in
# upload/. This closes that gap without adding any new credential or
# connection type: the source server counts its own accounts locally (as
# root, no API auth needed) and uploads a small JSON status file through
# the SAME SSH key + SFTP/rsync channel already set up for backups. The
# destination panel reads it and compares against what it actually
# received.
#
# IMPORTANT: this must run ON the cPanel/DirectAdmin server itself, as
# root - not on your own machine. It also requires that this server
# already has a working SSH key for sftp-backup-ui (via the panel's own
# "SSH Key ekle/değiştir" flow, or contrib/directadmin-sftp-backup-setup.sh
# for DirectAdmin) - this script does not generate a new one or touch your
# existing backup upload configuration at all.
#
#   scp contrib/account-count-check-setup.sh root@<kaynak-sunucu>:/root/
#   ssh root@<kaynak-sunucu>
#   ./account-count-check-setup.sh --hostname server.example.com --dest-host backup.example.com
#
# --dest-host/--dest-port refer to the sftp-backup-ui destination server
# and its SFTP port (default 22) - NOT whatever port/address you used to
# SSH into this source server to run the script.
#
# Verify anytime with:
#
#   ./account-count-check-setup.sh --test-only --hostname server.example.com --dest-host backup.example.com
#
# Safe to re-run: the cron entry is replaced deterministically from the
# same arguments, never duplicated.

set -euo pipefail

HOSTNAME_ARG=""
DEST_HOST=""
DEST_PORT="22"
DEST_USER=""
SCHED_HOUR="4"
SCHED_MINUTE="30"
TEST_ONLY="0"

usage() {
  cat <<EOF
Bu script'i cPanel/DirectAdmin sunucusuna SSH ile bağlanıp ORADA, root
olarak çalıştırın - kendi bilgisayarınızda değil.

Kullanım: $0 --hostname <ad> --dest-host <backup-sunucusu> [seçenekler]

Zorunlu:
  --hostname <ad>        Kaynak sunucu kimliği (mevcut SSH key dosya adı + varsayılan hedef kullanıcı adı)
  --dest-host <adres>    sftp-backup-ui'nin çalıştığı backup HEDEF sunucusunun adresi

Opsiyonel:
  --dest-port <port>     Hedef backup sunucusunun SFTP portu, varsayılan: 22
  --dest-user <ad>       Hedefteki chroot kullanıcı adı, varsayılan: --hostname ile aynı
  --schedule-hour <sa>   Cron saati (0-23), varsayılan: 4
  --schedule-minute <dk> Cron dakikası (0-59), varsayılan: 30
  --test-only            Cron'a dokunma, sadece sayımı bir kez çalıştırıp yükle
  -h, --help             Bu yardımı göster
EOF
}

log() { printf '==> %s\n' "$*"; }
die() { printf 'HATA: %s\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    --hostname) HOSTNAME_ARG="${2:-}"; shift 2 ;;
    --dest-host) DEST_HOST="${2:-}"; shift 2 ;;
    --dest-port) DEST_PORT="${2:-}"; shift 2 ;;
    --dest-user) DEST_USER="${2:-}"; shift 2 ;;
    --schedule-hour) SCHED_HOUR="${2:-}"; shift 2 ;;
    --schedule-minute) SCHED_MINUTE="${2:-}"; shift 2 ;;
    --test-only) TEST_ONLY="1"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "Bilinmeyen parametre: $1 (--help ile kullanım)" ;;
  esac
done

[ -n "${HOSTNAME_ARG}" ] || { usage; die "--hostname zorunlu"; }
[ -n "${DEST_HOST}" ] || { usage; die "--dest-host zorunlu"; }
[ -n "${DEST_USER}" ] || DEST_USER="${HOSTNAME_ARG}"

[ "$(id -u)" = "0" ] || die "root olarak çalıştırılmalı"

if [ -d /usr/local/cpanel ]; then
  PANEL="cpanel"
elif [ -d /usr/local/directadmin ]; then
  PANEL="directadmin"
else
  die "Ne /usr/local/cpanel ne /usr/local/directadmin bulundu - bu script yalnızca cPanel veya DirectAdmin sunucularında çalışır."
fi

KEY_PATH="/root/.ssh/backup_${HOSTNAME_ARG}"
[ -e "${KEY_PATH}" ] || die "SSH private key bulunamadı: ${KEY_PATH}. Önce bu sunucu için normal SFTP/SSH key kurulumunu tamamlayın (panelde \"SSH Key ekle/değiştir\", ya da DirectAdmin ise contrib/directadmin-sftp-backup-setup.sh) - bu script yeni bir anahtar üretmez, sadece bunu yeniden kullanır."

WRAPPER_PATH="/usr/local/bin/sftp-backup-ui-account-count.sh"

count_accounts() {
  case "${PANEL}" in
    cpanel) find /var/cpanel/users -mindepth 1 -maxdepth 1 -type f 2>/dev/null | wc -l ;;
    directadmin) find /usr/local/directadmin/data/users -mindepth 1 -maxdepth 1 -type d 2>/dev/null | wc -l ;;
  esac
}

run_check() {
  local count checked_at tmp_json
  count=$(count_accounts)
  checked_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  tmp_json=$(mktemp)
  printf '{"total_accounts": %d, "panel": "%s", "checked_at": "%s", "hostname": "%s"}\n' \
    "${count}" "${PANEL}" "${checked_at}" "${HOSTNAME_ARG}" > "${tmp_json}"

  log "Tespit edilen hesap sayısı (${PANEL}): ${count}"
  log "Yükleniyor: ${DEST_USER}@${DEST_HOST}:upload/.account-status.json"
  if rsync -az -e "ssh -i ${KEY_PATH} -p ${DEST_PORT} -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=accept-new" \
      "${tmp_json}" "${DEST_USER}@${DEST_HOST}:upload/.account-status.json"; then
    log "Yükleme başarılı."
    rm -f "${tmp_json}"
    return 0
  else
    rm -f "${tmp_json}"
    printf 'BAŞARISIZ. İpucu: public key panelde "%s" kullanıcısına eklenmemiş olabilir, veya bu kullanıcının SFTP erişimi henüz etkin değil.\n' "${DEST_USER}" >&2
    return 1
  fi
}

if [ "${TEST_ONLY}" = "1" ]; then
  if run_check; then exit 0; else exit 1; fi
fi

log "1/2 Wrapper script (${WRAPPER_PATH})"
cat > "${WRAPPER_PATH}" <<WRAPPER
#!/bin/bash
#
# sftp-backup-ui/contrib/account-count-check-setup.sh tarafından üretildi -
# elle düzenlemeyin, script'i tekrar çalıştırıp güncelleyin.
set -euo pipefail
PANEL="${PANEL}"
KEY_PATH="${KEY_PATH}"
DEST_HOST="${DEST_HOST}"
DEST_PORT="${DEST_PORT}"
DEST_USER="${DEST_USER}"
HOSTNAME_ARG="${HOSTNAME_ARG}"

case "\${PANEL}" in
  cpanel) COUNT=\$(find /var/cpanel/users -mindepth 1 -maxdepth 1 -type f 2>/dev/null | wc -l) ;;
  directadmin) COUNT=\$(find /usr/local/directadmin/data/users -mindepth 1 -maxdepth 1 -type d 2>/dev/null | wc -l) ;;
esac
CHECKED_AT=\$(date -u +%Y-%m-%dT%H:%M:%SZ)
TMP_JSON=\$(mktemp)
printf '{"total_accounts": %d, "panel": "%s", "checked_at": "%s", "hostname": "%s"}\n' \\
  "\${COUNT}" "\${PANEL}" "\${CHECKED_AT}" "\${HOSTNAME_ARG}" > "\${TMP_JSON}"

rsync -az -e "ssh -i \${KEY_PATH} -p \${DEST_PORT} -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=accept-new" \\
  "\${TMP_JSON}" "\${DEST_USER}@\${DEST_HOST}:upload/.account-status.json"
rm -f "\${TMP_JSON}"
WRAPPER
chmod 700 "${WRAPPER_PATH}"
chown root:root "${WRAPPER_PATH}"

log "2/2 Cron girişi (root crontab, günde bir, ${SCHED_HOUR}:${SCHED_MINUTE})"
CRON_MARKER="# sftp-backup-ui account-count: ${HOSTNAME_ARG} -> ${DEST_USER}@${DEST_HOST}"
CRON_LINE="${SCHED_MINUTE} ${SCHED_HOUR} * * * ${WRAPPER_PATH} ${CRON_MARKER}"
TMP_CRON=$(mktemp)
(crontab -l 2>/dev/null | grep -vF "${CRON_MARKER}") > "${TMP_CRON}" || true
printf '%s\n' "${CRON_LINE}" >> "${TMP_CRON}"
crontab "${TMP_CRON}"
rm -f "${TMP_CRON}"

log "Bağlantı testi"
if run_check; then
  TEST_RESULT="başarılı"
else
  TEST_RESULT="başarısız - yukarıdaki ipucuna bakın"
fi

echo
log "Tamamlandı: ${HOSTNAME_ARG} (${PANEL}) -> ${DEST_USER}@${DEST_HOST}:${DEST_PORT}, günde bir ${SCHED_HOUR}:${SCHED_MINUTE} (test: ${TEST_RESULT})"
