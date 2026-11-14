# 配方默认在 justfile 所在目录执行。
set positional-arguments := true

default: help

# 显示开发命令
help:
    @just --justfile "{{justfile()}}" --list

# 编译无 CGO 的可执行文件
build:
    mkdir -p bin
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/try-go ./cmd/try-go

# 运行；例如 just run --help
run *args: build
    ./bin/try-go "$@"

# 运行测试
test:
    CGO_ENABLED=0 go test ./...

# 只安装二进制，不修改 Shell 配置
install: build
    install -d "$HOME/.local/bin"
    install -m 755 bin/try-go "$HOME/.local/bin/try-go"
