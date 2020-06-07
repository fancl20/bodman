BUILD_TAGS=-tags containers_image_openpgp,exclude_graphdriver_btrfs,exclude_graphdriver_devicemapper
default all:
	go build $(BUILD_TAGS)
update:
	go get $(BUILD_TAGS) -u ./... && go mod tidy
