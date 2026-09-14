# ========== 构建阶段 ==========
FROM golang:1.25-alpine AS builder

ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o server ./cmd/main.go

# ========== 运行阶段 ==========
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Shanghai

RUN adduser -D -h /home/app appuser
WORKDIR /app

COPY --from=builder /build/server .

EXPOSE 8080

USER appuser
ENTRYPOINT ["./server"]
