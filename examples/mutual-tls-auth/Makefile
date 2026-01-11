# SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
# SPDX-License-Identifier: MIT

turn_realm ?= pion.ly

turn_server_allocation_ip ?= 127.0.0.1
turn_server_host ?= localhost
turn_server_port ?= 5349

ca_cert_cn ?= Pion Certificate Authority Root
ca_cert_lifetime_days ?= 365
ca_cert ?= ca.crt
ca_key ?= ca.key

client_cert_cn ?= turn-client
client_cert_lifetime_days ?= 1
client_cert ?= client.crt
client_key ?= client.key

server_cert_cn ?= turn-server
server_cert_lifetime_days ?= 1
server_cert ?= server.crt
server_key ?= server.key

.PHONY: help
help: ## Print help menu
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.PHONY: generate-ca-cert
generate-ca-cert: ## Generate CA certificate and private key
	@openssl req -new -x509 \
		-days "$(ca_cert_lifetime_days)" \
		-keyout "$(ca_key)" \
		-out "$(ca_cert)" \
		-nodes \
		-subj "/O=$(turn_realm)/CN=$(ca_cert_cn)"

.PHONY: generate-client-cert
generate-client-cert: ## Generate client certificate and private key
	@openssl req -new -newkey rsa:2048 -nodes \
		-keyout "$(client_key)" \
		-subj "/O=$(turn_realm)/CN=$(client_cert_cn)" | \
	openssl x509 -req \
		-CA "$(ca_cert)" -CAkey "$(ca_key)" -set_serial $$RANDOM \
		-out "$(client_cert)" -days "$(client_cert_lifetime_days)"

.PHONY: generate-server-cert
generate-server-cert: ## Generate server certificate and private key
	@openssl req -new -newkey rsa:2048 -nodes \
		-keyout "$(server_key)" \
		-subj "/O=$(turn_realm)/CN=$(server_cert_cn)" \
		-addext "subjectAltName=DNS:$(turn_server_host)" | \
	openssl x509 -req \
		-CA "$(ca_cert)" -CAkey "$(ca_key)" -set_serial $$RANDOM \
		-out "$(server_cert)" -days "$(server_cert_lifetime_days)" \
		-copy_extensions copyall

.PHONY: generate-certs
generate-certs: generate-ca-cert generate-client-cert generate-server-cert ## Generate ALL certificates and keys (CA, client, server)

.PHONY: run-server
run-server: ## Run the TURN server with TLS certificate authentication
	@go run ./turn-server/main.go \
		-public-ip "$(turn_server_allocation_ip)" \
		-port "$(turn_server_port)" \
		-realm "$(turn_realm)" \
		-cert "$(server_cert)" \
		-key "$(server_key)" \
		-ca "$(ca_cert)"

.PHONY: run-client
run-client: ## Run the TURN client with TLS certificate authentication
	@go run ./turn-client/main.go \
		-host "$(turn_server_host)" \
		-port "$(turn_server_port)" \
		-user "$(client_cert_cn)" \
		-realm "$(turn_realm)" \
		-cert "$(client_cert)" \
		-key "$(client_key)" \
		-ca "$(ca_cert)" \
		-ping

.PHONY: clean
clean: ## Deletes ALL certificates and key files (CA, client, server)
	@rm $(ca_cert) || true
	@rm $(ca_key) || true
	@rm $(client_cert) || true
	@rm $(client_key) || true
	@rm $(server_cert) || true
	@rm $(server_key) || true
	@echo "Ensured no certificate and key files are present anymore."
