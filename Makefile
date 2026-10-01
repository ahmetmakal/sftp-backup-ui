APP_NAME       := sftp-backup-ui
SERVICE_NAME   := $(APP_NAME).service

REMOTE_USER    ?= root
REMOTE_HOST    ?= backup.example.com
REMOTE_PORT    ?= 22
REMOTE_DIR     := /opt/$(APP_NAME)
REMOTE_ENV_DIR := /etc/$(APP_NAME)

GOARCH         ?= amd64

SSH_TARGET     := $(REMOTE_USER)@$(REMOTE_HOST)
SSH            := ssh -p $(REMOTE_PORT) $(SSH_TARGET)
RSYNC          := rsync -az -e "ssh -p $(REMOTE_PORT)"

.PHONY: css build deploy restart status logs ssh clean

node_modules: package.json
	npm install
	@touch node_modules

css: node_modules
	npx tailwindcss -i ./web/tailwind.css -o ./static/app.css --minify

build: css
	GOOS=linux GOARCH=$(GOARCH) go build -o bin/$(APP_NAME) ./cmd/server

deploy: build
	$(SSH) 'mkdir -p $(REMOTE_DIR)/bin $(REMOTE_DIR)/contrib $(REMOTE_ENV_DIR)'
	$(RSYNC) bin/$(APP_NAME) $(SSH_TARGET):$(REMOTE_DIR)/bin/$(APP_NAME)
	$(RSYNC) --delete templates/ $(SSH_TARGET):$(REMOTE_DIR)/templates/
	$(RSYNC) --delete static/ $(SSH_TARGET):$(REMOTE_DIR)/static/
	$(RSYNC) contrib/ $(SSH_TARGET):$(REMOTE_DIR)/contrib/
	$(RSYNC) deploy/env.example $(SSH_TARGET):$(REMOTE_DIR)/env.example
	$(RSYNC) deploy/$(SERVICE_NAME) $(SSH_TARGET):/etc/systemd/system/$(SERVICE_NAME)
	$(SSH) 'chmod +x $(REMOTE_DIR)/bin/$(APP_NAME); \
		test -f $(REMOTE_ENV_DIR)/env || install -m 600 $(REMOTE_DIR)/env.example $(REMOTE_ENV_DIR)/env; \
		systemctl daemon-reload; \
		systemctl enable --now $(SERVICE_NAME); \
		systemctl restart $(SERVICE_NAME)'
	@echo ""
	@echo "Deploy tamam. İlk kurulumsa şimdi $(REMOTE_ENV_DIR)/env dosyasını sunucuda düzenleyip"
	@echo "ADMIN_PASSWORD'u değiştirin, sonra: make restart"

restart:
	$(SSH) 'systemctl restart $(SERVICE_NAME)'

status:
	$(SSH) 'systemctl status $(SERVICE_NAME) --no-pager'

logs:
	$(SSH) 'journalctl -u $(SERVICE_NAME) -f'

ssh:
	$(SSH)

clean:
	rm -rf bin
