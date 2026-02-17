ARTIFACTS ?= $(HOME)/fc-artifacts
ROOTFS ?= $(ARTIFACTS)/ubuntu-24.04.ext4

.PHONY: artifacts patch build run run-net

artifacts:
	./scripts/get-artifacts.sh $(ARTIFACTS)

patch:
	./scripts/patch-rootfs.sh $(ROOTFS)

build:
	go build ./cmd/fc-run
	go build ./cmd/fc-weather-demo

run:
	./cmd/fc-run/fc-run -fc $(ARTIFACTS)/firecracker -kernel $(ARTIFACTS)/vmlinux -rootfs $(ROOTFS) -timeout 30s -keep

run-net:
	sudo ./cmd/fc-run/fc-run -fc $(ARTIFACTS)/firecracker -kernel $(ARTIFACTS)/vmlinux -rootfs $(ROOTFS) -timeout 30s -keep -net -mmds
