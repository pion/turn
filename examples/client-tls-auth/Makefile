# SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
# SPDX-License-Identifier: MIT

turn_realm ?= pion.ly

turn_server_allocation_ip ?= 127.0.0.1
turn_server_host ?= localhost
turn_server_port ?= 5349

ca_cert_cn ?= Pion Certificate Authority Root
ca_cert_lifetime_seconds ?= $(shell echo $$((60 * 60 * 24 * 365 * 3)))
ca_cert ?= ca.crt
ca_key ?= ca.key

client_cert_cn ?= turn-client
client_cert_lifetime_seconds ?= $(shell echo $$((60 * 60 * 12)))
client_cert ?= client.crt
client_key ?= client.key

server_cert_cn ?= turn-server
server_cert_lifetime_seconds ?= $(shell echo $$((60 * 60 * 12)))
server_cert ?= server.crt
server_key ?= server.key

.PHONY: help
help: ## Print help menu
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.PHONY: generate-ca-cert
generate-ca-cert: ## Generate CA certificate and private key
	@go run ./scripts/generate-ca-cert/main.go \
		-cert-org "$(turn_realm)" \
		-cert-cn "$(ca_cert_cn)" \
		-cert-lifetime-seconds "$(ca_cert_lifetime_seconds)" \
		-cert-out "$(ca_cert)" \
		-key-out "$(ca_key)"

.PHONY: generate-client-cert
generate-client-cert: ## Generate client certificate and private key
	@go run ./scripts/generate-and-sign-cert/main.go \
		-cert-org "$(turn_realm)" \
		-cert-cn "$(client_cert_cn)" \
		-cert-lifetime-seconds "$(client_cert_lifetime_seconds)" \
		-cert-out "$(client_cert)" \
		-key-out "$(client_key)" \
		-ca-cert "$(ca_cert)" \
		-ca-key "$(ca_key)"

.PHONY: generate-server-cert
generate-server-cert: ## Generate server certificate and private key
	@go run ./scripts/generate-and-sign-cert/main.go \
		-cert-org "$(turn_realm)" \
		-cert-cn "$(server_cert_cn)" \
		-cert-sans "$(turn_server_host)" \
		-cert-lifetime-seconds "$(server_cert_lifetime_seconds)" \
		-cert-out "$(server_cert)" \
		-key-out "$(server_key)" \
		-ca-cert "$(ca_cert)" \
		-ca-key "$(ca_key)"

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
