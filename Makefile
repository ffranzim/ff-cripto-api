-include .env
export

IMAGE ?= localhost/ff-cripto:latest

VPS        ?= franzim@217.76.49.188
REMOTE_DIR ?= apps/ff-cripto-api
QUADLET    := ~/.config/containers/systemd

.PHONY: tidy sqlc test run migrate build image deploy logs status

tidy:     ## atualiza go.mod/go.sum
	go mod tidy

sqlc:     ## regenera internal/repository/sqlcdb a partir de db/queries
	sqlc generate

test:
	go test ./...

run:
	go run ./cmd/api

migrate:  ## aplica migrations pendentes (goose embutido)
	go run ./cmd/api migrate

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/api ./cmd/api

image:
	podman build -t $(IMAGE) .

# ---- VPS ----
# O secret "ff-cripto" é criado pelo repo de
# infra (vps/contabo: make secrets), a partir do .env de lá.

deploy: test  ## envia o código, builda a imagem na VPS e (re)inicia o serviço
	ssh $(VPS) 'mkdir -p ~/$(REMOTE_DIR)'
	rsync -av --delete --exclude .git --exclude .env --exclude bin --exclude .tmp-claude ./ $(VPS):~/$(REMOTE_DIR)/
	ssh $(VPS) 'set -e; cd ~/$(REMOTE_DIR) && \
		podman build -t $(IMAGE) . && \
		mkdir -p $(QUADLET) && \
		install -m 644 deploy/ff-cripto.kube deploy/ff-cripto.yaml $(QUADLET)/ && \
		systemctl --user daemon-reload && \
		systemctl --user restart ff-cripto && \
		podman pod ps --filter name=ff-cripto'

tunnel:   ## Postgres da VPS em localhost:5432 (deixe aberto num terminal e rode make run em outro)
	ssh -N -L 5432:127.0.0.1:5432 $(VPS)

logs:     ## logs da API na VPS
	ssh $(VPS) 'journalctl --user -u ff-cripto -n 100 --no-pager'

status:
	ssh $(VPS) 'systemctl --user --no-pager --lines=0 status ff-cripto; podman pod ps --filter name=ff-cripto'
