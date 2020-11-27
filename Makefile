build:
	go build -ldflags="-s -w"

shrink:
	# https://github.com/upx/upx/releases/
	upx --brute replicator

deploy:
	parallel-rsync -X --progress -h hosts.txt -l laurence ./replicator /home/laurence