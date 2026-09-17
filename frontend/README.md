# frontend

按目标形态分目录。**MVP 使用 H5**，构建产物直接输出到 Go 服务的内嵌目录。

## 目录

```text
h5/           MVP 前端（Vue3 + Vite + TS，与 omnibot 同栈）—— 脚手架待初始化
app/          Flutter 客户端（M3 之后）
miniprogram/  微信小程序（远期）
```

## H5 与 Go 的连接方式（沿用 omnibot 模式）

```text
frontend/h5/vite.config.ts  →  build.outDir 指向 ../../backend/web/dist
backend/web/embed.go        →  //go:embed all:dist 打包进二进制
worldd                      →  静态路由 / 承载页面，/api/* 承载接口
```

单二进制自包含，无外部静态文件依赖。
