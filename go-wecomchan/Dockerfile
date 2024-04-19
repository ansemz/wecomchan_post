# 使用带有Go环境的Alpine基础镜像进行构建
FROM golang:1.16.5-alpine3.13 AS gobuilder

# 替换为国内源，并安装必要的软件包
RUN apk add --no-cache ca-certificates && \
    update-ca-certificates

# 设置工作目录并复制源代码
WORKDIR /go/src/app
COPY . .

# 编译应用
RUN go build -o wecomchan

# 使用Alpine作为最终镜像
FROM alpine:latest

# 安装时区支持和SSL证书
RUN apk add --no-cache ca-certificates tzdata && \
    update-ca-certificates && \
    cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone

# 设置工作目录
WORKDIR /root

# 从构建阶段复制编译好的应用和证书
COPY --from=gobuilder /go/src/app/wecomchan .
COPY --from=gobuilder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# 暴露端口
EXPOSE 8080

# 容器启动时执行的命令
CMD ["./wecomchan"]
