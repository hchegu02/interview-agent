# Vite API 代理修复

## 1. 变更概述

本次修复前端开发服务器下的 API 404 问题。开发时从 `http://127.0.0.1:5173` 打开页面，前端使用相对路径请求 `/api/profile/analyze`，但 `web/vite.config.ts` 没有配置 `/api` 代理，Vite dev server 会返回 404/HTML，前端按 JSON 解析后出现 `Unexpected token '<'`。

修复方式是在 Vite dev server 中把 `/api` 请求代理到后端 `http://127.0.0.1:8080`。这只影响前端开发环境，不影响生产构建产物和后端嵌入式静态资源服务。

## 2. 变更文件

- `web/vite.config.ts`：新增 Vite dev server `/api` proxy。

## 3. 函数级说明

### `defineConfig(...)`

位置：`web/vite.config.ts`

作用：定义 Vite 前端开发和构建配置。

输入：Vite 配置对象。

输出：Vite 配置。

副作用：开发模式下，所有以 `/api` 开头的请求会由 Vite 代理到 `http://127.0.0.1:8080`。如果后端未启动，前端 API 请求会失败并暴露后端连接错误，而不是收到 Vite 的 HTML/404。

错误处理：Vite proxy 本身不改变后端响应；后端错误会原样返回给前端调用方。

主要逻辑和行为变化：`npm --prefix web run dev` 启动后，页面中的 `fetch("/api/...")` 和 `EventSource("/api/...")` 会命中 Go 后端服务。

## 4. 调用链

用户在浏览器打开 Vite dev server 页面
-> React 页面触发 `web/src/main.tsx` 中的 `analyze`
-> `apiClient.analyzeProfile`
-> `api(...)`
-> `fetch("/api/profile/analyze", ...)`
-> Vite dev server `/api` proxy
-> Go 后端 `cmd/server.main`
-> `httpapi.Server.Router`
-> `POST /api/profile/analyze`
-> `Server.analyzeProfile`

## 5. 数据流

前端把 JD 和简历文本作为 JSON 请求体发送到 `/api/profile/analyze`。Vite proxy 不修改请求体，只转发到 `127.0.0.1:8080`。后端返回 JSON 后，前端 `api(...)` 读取文本并解析为 JSON。

## 6. 依赖与副作用

- 新增 Vite dev server proxy 配置。
- 不新增 npm 依赖。
- 不修改后端 API。
- 不修改生产环境配置。
- 开发环境要求后端先运行在 `127.0.0.1:8080`，或后续再扩展为可配置代理目标。

## 7. 测试

已执行：

```powershell
npm --prefix web run test
npm --prefix web run build
```

结果：

- `npm --prefix web run test`：8 个测试文件、31 个测试通过。
- `npm --prefix web run build`：`tsc -b && vite build` 通过。

手工验证建议：

```powershell
go run ./cmd/server -config config/config.yaml.example
npm --prefix web run dev
```

然后在浏览器重新打开 `http://127.0.0.1:5173`，点击 Step 2 的分析按钮。

## 8. 风险

- 兼容性：只影响 Vite dev server，不影响生产构建。
- 环境：如果后端不在 `127.0.0.1:8080`，开发代理会失败。当前项目 example 配置默认监听 `:8080`，符合该配置。
- 安全：代理只在本地开发服务器生效，不提交 token 或私有配置。
