.PHONY: check test race lint fmt cover clean

# 交作业前跑这个。全绿再叫我 review。
check: fmt lint race

test:
	go test ./...

# -race 是竞态检测器，第 2 周开始它会救你的命。养成习惯从现在开始。
race:
	go test -race ./...

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...
	golangci-lint fmt ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

clean:
	go clean -testcache
	rm -f coverage.out
