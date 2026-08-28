#!/usr/bin/env bash
#
# directadmin-sftp-backup-setup.sh - idempotent setup of SSH-key SFTP backup
# uploads on a DirectAdmin source server, targeting an sftp-backup-ui
# destination user.
#
# DirectAdmin's native Admin Backup/Transfer only supports FTP/FTPS with a
# password. This installs a custom upload hook
# (/usr/local/directadmin/scripts/custom/ftp_upload.php - a bash script; the
# .php extension is just DirectAdmin's override convention) that uploads via
# SFTP using an SSH key instead, and wires up backup.conf + a cron entry to
# use it.
#
# IMPORTANT: this must run ON the DirectAdmin server itself, as root - not
# on your own machine. SSH into the DirectAdmin server first, copy the
# script there (or paste it in), then run it locally on that server:
#
#   scp contrib/directadmin-sftp-backup-setup.sh root@<DA sunucusu>:/root/
#   ssh root@<DA sunucusu>
#   ./directadmin-sftp-backup-setup.sh --hostname server.example.com --dest-host backup.example.com
#
# --dest-host/--dest-port refer to the sftp-backup-ui destination server and
# its SFTP port (default 22) - NOT whatever port/address you used to SSH
# into this DirectAdmin server to run the script.
#
# After the first run, paste the printed public key into the sftp-backup-ui
# panel for the matching destination user ("SSH Key ekle/değiştir"), then
# verify with:
#
#   ./directadmin-sftp-backup-setup.sh --test-only --hostname server.example.com --dest-host backup.example.com
#
# Safe to re-run: keygen is skipped if the key already exists, and the hook
# script / backup.conf / cron entry are all regenerated deterministically
# from the same arguments.

set -euo pipefail

HOSTNAME_ARG=""
DEST_HOST=""
DEST_PORT="22"
DEST_USER=""
DEST_PATH="/upload"
SCHED_HOUR="3"
SCHED_MINUTE="0"
TEST_ONLY="0"

usage() {
  cat <<EOF
Bu script'i DirectAdmin sunucusuna SSH ile bağlanıp ORADA, root olarak
çalıştırın - kendi bilgisayarınızda değil.

Kullanım: $0 --hostname <ad> --dest-host <backup-sunucusu> [seçenekler]

Zorunlu:
  --hostname <ad>        Kaynak sunucu kimliği (SSH key dosya adı + varsayılan hedef kullanıcı adı)
  --dest-host <adres>    sftp-backup-ui'nin çalıştığı backup HEDEF sunucusunun adresi

Opsiyonel:
  --dest-port <port>     Hedef backup sunucusunun SFTP portu (bu DirectAdmin sunucusuna
                         bağlanmak için kullandığınız SSH portu DEĞİL). Varsayılan: 22
  --dest-user <ad>       Hedefteki chroot kullanıcı adı, varsayılan: --hostname ile aynı
  --dest-path <yol>      Varsayılan: /upload
  --schedule-hour <sa>   Cron saati (0-23), varsayılan: 3
  --schedule-minute <dk> Cron dakikası (0-59), varsayılan: 0
  --test-only            Dosya yazma adımlarını atla, sadece bağlantı testini çalıştır
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
    --dest-path) DEST_PATH="${2:-}"; shift 2 ;;
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
[ -d /usr/local/directadmin ] || die "/usr/local/directadmin bulunamadı. Bu script'i kendi bilgisayarınızda değil, SSH ile DirectAdmin sunucusuna bağlanıp ORADA çalıştırmanız gerekiyor."

KEY_PATH="/root/.ssh/backup_${HOSTNAME_ARG}"
DA_SSH_DIR="/usr/local/directadmin/data/admin/.backup_ssh"
DA_KEY_PATH="${DA_SSH_DIR}/backup_${HOSTNAME_ARG}"
DA_KNOWN_HOSTS="${DA_SSH_DIR}/known_hosts"
HOOK_PATH="/usr/local/directadmin/scripts/custom/ftp_upload.php"
BACKUP_CONF="/usr/local/directadmin/data/admin/backup.conf"
CRONS_LIST="/usr/local/directadmin/data/admin/backup_crons.list"

run_connectivity_test() {
  log "Bağlantı testi: sftp -i ${DA_KEY_PATH} -P ${DEST_PORT} ${DEST_USER}@${DEST_HOST}"
  if [ ! -e "${DA_KEY_PATH}" ]; then
    printf 'BAŞARISIZ: özel anahtar bulunamadı: %s (önce kurulum adımlarını çalıştırın, --test-only olmadan)\n' "${DA_KEY_PATH}" >&2
    return 1
  fi
  local output
  if output=$(sftp -i "${DA_KEY_PATH}" -P "${DEST_PORT}" \
      -o BatchMode=yes -o ConnectTimeout=10 \
      -o UserKnownHostsFile="${DA_KNOWN_HOSTS}" \
      -o StrictHostKeyChecking=accept-new \
      "${DEST_USER}@${DEST_HOST}" <<< "ls ${DEST_PATH}" 2>&1); then
    log "Bağlantı başarılı."
    printf '%s\n' "${output}"
    return 0
  else
    printf 'BAŞARISIZ:\n%s\n' "${output}" >&2
    printf 'İpucu: public key henüz sftp-backup-ui panelinde "%s" kullanıcısına eklenmemiş olabilir (SSH Key ekle/değiştir), veya %s yolu hedefte mevcut değil.\n' "${DEST_USER}" "${DEST_PATH}" >&2
    return 1
  fi
}

if [ "${TEST_ONLY}" = "1" ]; then
  if run_connectivity_test; then exit 0; else exit 1; fi
fi

log "1/6 SSH anahtarı"
if [ -e "${KEY_PATH}" ]; then
  log "Zaten var, atlanıyor: ${KEY_PATH}"
else
  mkdir -p "$(dirname "${KEY_PATH}")"
  ssh-keygen -t ed25519 -f "${KEY_PATH}" -N "" -C "backup_${HOSTNAME_ARG}" >/dev/null
  log "Oluşturuldu: ${KEY_PATH}"
fi

echo
echo "=============================================================="
echo " Aşağıdaki public key'i sftp-backup-ui panelinde"
echo " \"${DEST_USER}\" kullanıcısına ekleyin (SSH Key ekle/değiştir):"
echo "=============================================================="
cat "${KEY_PATH}.pub"
echo "=============================================================="
echo

log "2/6 diradmin için anahtar kopyası (${DA_SSH_DIR})"
mkdir -p "${DA_SSH_DIR}"
cp "${KEY_PATH}" "${DA_KEY_PATH}"
touch "${DA_KNOWN_HOSTS}"
chown -R diradmin:diradmin "${DA_SSH_DIR}"
chmod 700 "${DA_SSH_DIR}"
chmod 600 "${DA_SSH_DIR}"/*

log "3/6 Upload hook'u (${HOOK_PATH})"
if [ -e "${HOOK_PATH}" ]; then
  cp -a "${HOOK_PATH}" "${HOOK_PATH}.bak-$(date +%Y%m%d%H%M%S)"
  log "Mevcut dosya yedeklendi."
fi
mkdir -p "$(dirname "${HOOK_PATH}")"
cat > "${HOOK_PATH}" <<'HOOK'
#!/bin/bash
#
# sftp-backup-ui/contrib/directadmin-sftp-backup-setup.sh tarafından üretildi - elle
# düzenlemeyin, script'i tekrar çalıştırıp güncelleyin.
#
# DirectAdmin Admin Backup/Transfer, FTP/FTPS yerine SSH-key ile SFTP
# kullanarak yedeği yükler. DirectAdmin şu değişkenleri env olarak geçer:
# ftp_ip, ftp_username, ftp_password, ftp_path, ftp_local_file,
# ftp_remote_file, ftp_port (ftp_port bazen boş geldiği için burada sabit
# bir değer kullanılıyor).

SFTP_KEY="__DA_KEY_PATH__"
SFTP_KNOWN_HOSTS="__DA_KNOWN_HOSTS__"
SFTP_BIN="/usr/bin/sftp"
SFTP_PORT=__DEST_PORT__

if [ ! -e "${ftp_local_file}" ]; then
	echo "Cannot find backup file ${ftp_local_file} to upload"
	exit 11
fi
if [ ! -e "${SFTP_KEY}" ]; then
	echo "SSH private key not found: ${SFTP_KEY}"
	exit 12
fi

# Build a chain of "-mkdir" commands (leading "-" = ignore errors if it
# already exists) so nested remote paths get created as needed.
REMOTE_PATH="${ftp_path}"
MKDIR_CMDS=""
CUR=""
IFS='/' read -ra PARTS <<< "${REMOTE_PATH}"
for P in "${PARTS[@]}"; do
	[ -z "${P}" ] && continue
	CUR="${CUR}/${P}"
	MKDIR_CMDS="${MKDIR_CMDS}-mkdir \"${CUR}\"
"
done

BATCH=$(mktemp)
chmod 600 "${BATCH}"
{
	printf '%s' "${MKDIR_CMDS}"
	echo "put \"${ftp_local_file}\" \"${REMOTE_PATH}/${ftp_remote_file}\""
} > "${BATCH}"

OUTPUT=$("${SFTP_BIN}" -i "${SFTP_KEY}" -P "${SFTP_PORT}" \
	-o BatchMode=yes -o ConnectTimeout=15 \
	-o UserKnownHostsFile="${SFTP_KNOWN_HOSTS}" \
	-o StrictHostKeyChecking=accept-new \
	-b "${BATCH}" \
	"${ftp_username}@${ftp_ip}" 2>&1)
RET=$?
rm -f "${BATCH}"

echo "${OUTPUT}"
[ "${RET}" -ne 0 ] && echo "sftp upload failed with exit code ${RET}"
exit ${RET}
HOOK
sed -i \
  -e "s#__DA_KEY_PATH__#${DA_KEY_PATH}#g" \
  -e "s#__DA_KNOWN_HOSTS__#${DA_KNOWN_HOSTS}#g" \
  -e "s#__DEST_PORT__#${DEST_PORT}#g" \
  "${HOOK_PATH}"
# DirectAdmin runs the backup task (and this hook) as the unprivileged
# "diradmin" system user, not root - and diradmin isn't a member of the
# "root" group, so root:root 750 leaves it completely unreadable ("Unable
# to read ... ftp_upload.php" at backup time even though the file is
# correctly in place). The hook has no secrets in it, so world-readable is
# the simple, portable fix instead of guessing diradmin's actual gid/group
# name across different DirectAdmin installs.
chmod 755 "${HOOK_PATH}"
chown root:root "${HOOK_PATH}"

log "4/6 backup.conf (${BACKUP_CONF})"
touch "${BACKUP_CONF}"
TMP_CONF=$(mktemp)
grep -Ev '^(ftp_ip|ftp_path|ftp_port|ftp_secure|ftp_username)=' "${BACKUP_CONF}" > "${TMP_CONF}" || true
{
  cat "${TMP_CONF}"
  echo "ftp_ip=${DEST_HOST}"
  echo "ftp_path=${DEST_PATH}"
  echo "ftp_port=${DEST_PORT}"
  echo "ftp_secure=ftps"
  echo "ftp_username=${DEST_USER}"
} > "${BACKUP_CONF}"
rm -f "${TMP_CONF}"

log "5/6 backup_crons.list (${CRONS_LIST})"
touch "${CRONS_LIST}"
ENCODED_PATH=$(printf '%s' "${DEST_PATH}" | sed 's#/#%2F#g')
# "who=all" is required for DirectAdmin to schedule this as an All-Users
# backup job. Without it, the job is silently created in "Selected Users"
# mode with zero accounts selected (confirmed by inspecting the "who" field
# read by the Evolution skin's backup-jobs monitor UI, which distinguishes
# who.who === "all" vs "selected" - this field is undocumented and absent
# from the reference runbook this script was built from).
CRON_VALUE="action=backup&append%5Fto%5Fpath=nothing&database%5Fdata%5Faware=yes&dayofmonth=%2A&dayofweek=%2A&email%5Fdata%5Faware=yes&ftp%5Fip=${DEST_HOST}&ftp%5Fpassword=&ftp%5Fpath=${ENCODED_PATH}&ftp%5Fport=${DEST_PORT}&ftp%5Fsecure=ftps&ftp%5Fusername=${DEST_USER}&hour=${SCHED_HOUR}&minute=${SCHED_MINUTE}&month=%2A&owner=admin&trash%5Faware=yes&type=admin&value=multiple&when=now&where=ftp&who=all"

TMP_CRONS=$(mktemp)
MATCH_TOKEN="ftp%5Fusername=${DEST_USER}&"
FOUND=0
MAX_ID=0
while IFS= read -r LINE || [ -n "${LINE}" ]; do
  [ -z "${LINE}" ] && continue
  ID="${LINE%%=*}"
  case "${ID}" in ''|*[!0-9]*) : ;; *) [ "${ID}" -gt "${MAX_ID}" ] && MAX_ID="${ID}" ;; esac
  if printf '%s' "${LINE}" | grep -qF "${MATCH_TOKEN}"; then
    printf '%s=%s\n' "${ID}" "${CRON_VALUE}" >> "${TMP_CRONS}"
    FOUND=1
  else
    printf '%s\n' "${LINE}" >> "${TMP_CRONS}"
  fi
done < "${CRONS_LIST}"

if [ "${FOUND}" = "0" ]; then
  NEXT_ID=$((MAX_ID + 1))
  printf '%s=%s\n' "${NEXT_ID}" "${CRON_VALUE}" >> "${TMP_CRONS}"
  log "Yeni cron girişi eklendi (id=${NEXT_ID}, ${SCHED_HOUR}:${SCHED_MINUTE} her gün)."
else
  log "Mevcut cron girişi güncellendi."
fi
mv "${TMP_CRONS}" "${CRONS_LIST}"
chown diradmin:diradmin "${CRONS_LIST}"
chmod 600 "${CRONS_LIST}"

log "6/6 Bağlantı testi"
if run_connectivity_test; then
  TEST_RESULT="başarılı"
else
  TEST_RESULT="başarısız - yukarıdaki ipucuna bakın"
fi

echo
log "Tamamlandı: ${HOSTNAME_ARG} -> ${DEST_USER}@${DEST_HOST}:${DEST_PORT}${DEST_PATH} (bağlantı testi: ${TEST_RESULT})"
if [ "${TEST_RESULT}" != "başarılı" ]; then
  log "Public key'i panele ekledikten sonra tekrar test etmek için:"
  log "  $0 --test-only --hostname ${HOSTNAME_ARG} --dest-host ${DEST_HOST} --dest-port ${DEST_PORT} --dest-user ${DEST_USER} --dest-path ${DEST_PATH}"
fi
