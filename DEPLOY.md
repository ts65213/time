# 部署文档（time）

本文档用于将当前仓库发布到 Linux 服务器，并通过 `systemd` 常驻运行。

## 1. 部署方式说明

- 后端：Go 单体服务（启动时自动执行数据库迁移与初始化）
- 前端：`vite build` 产物，放在 `frontend/dist`
- 对外入口：后端同时提供 API 与静态页面

代码依据：
- 环境变量读取与启动流程：[main.go](file:///c:/0/00%20%20code/time/main.go#L126-L197)
- 前端构建命令：[package.json](file:///c:/0/00%20%20code/time/frontend/package.json#L6-L9)
- 环境变量示例：[.env.example](file:///c:/0/00%20%20code/time/.env.example#L1-L4)

---

## 2. 环境要求

### 2.1 本地（发布机）

- Go（用于构建 Linux 二进制）
- Node.js + npm（用于构建前端）
- ssh / scp（用于上传）

### 2.2 服务器

- Linux x86_64
- PostgreSQL 可访问
- systemd

---

## 3. 必要环境变量

在服务器应用目录（如 `/opt/time`）放置 `.env`：

```env
DB_DSN=postgres://postgres:your_password@127.0.0.1:5432/postgres?sslmode=disable
ADMIN_USERNAME=ts65213
ADMIN_PASSWORD=your_password
PORT=5174
```

说明：
- `DB_DSN` 必填，否则程序启动失败。
- `PORT` 不填时默认 `5174`。

---

## 4. 目录约定（推荐）

```text
/opt/time/
  ├─ .env
  ├─ timeapp                  # 当前运行二进制
  ├─ frontend/
  │   └─ dist/                # 当前运行前端静态资源
  └─ releases/
      └─ 20260322182217/      # 每次发布一个目录（时间戳）
          ├─ timeapp
          └─ frontend/dist/
```

---

## 5. 首次部署（服务器）

以下步骤只需做一次：

1) 创建目录和运行用户（示例）：

```bash
sudo mkdir -p /opt/time/frontend /opt/time/releases
sudo useradd -r -s /sbin/nologin timeapp || true
sudo chown -R timeapp:timeapp /opt/time
```

2) 写入 `/opt/time/.env`（按第 3 节配置）。

3) 创建 systemd 服务 `/etc/systemd/system/timeapp.service`：

```ini
[Unit]
Description=timeapp
After=network.target

[Service]
Type=simple
User=timeapp
Group=timeapp
WorkingDirectory=/opt/time
ExecStart=/opt/time/timeapp
Restart=always
RestartSec=3
EnvironmentFile=/opt/time/.env

[Install]
WantedBy=multi-user.target
```

4) 启用服务：

```bash
sudo systemctl daemon-reload
sudo systemctl enable timeapp
```

---

## 6. 日常发布（Windows PowerShell）

以下命令在仓库根目录执行（请替换服务器信息与密钥路径）：

```powershell
$Server = "root@47.242.41.252"
$Key    = "C:\Users\Administrator\.ssh\abc.pem"
$Ts     = Get-Date -Format yyyyMMddHHmmss

# 1) 构建前端
cd frontend
npm run build
cd ..

# 2) 构建 Linux 后端
$env:GOOS='linux'
$env:GOARCH='amd64'
go build -o timeapp_linux .
Remove-Item Env:GOOS
Remove-Item Env:GOARCH

# 3) 上传发布包（推荐用 tar，避免 scp 目录问题）
tar -czf dist_upload.tgz -C frontend dist
ssh -i $Key $Server "mkdir -p /opt/time/releases/$Ts/frontend"
scp -i $Key timeapp_linux $Server:/opt/time/releases/$Ts/timeapp
scp -i $Key dist_upload.tgz $Server:/opt/time/releases/$Ts/dist_upload.tgz
ssh -i $Key $Server "cd /opt/time/releases/$Ts && tar -xzf dist_upload.tgz -C frontend && rm -f dist_upload.tgz"

# 4) 切换线上版本并重启
ssh -i $Key $Server "set -e; install -m 755 /opt/time/releases/$Ts/timeapp /opt/time/timeapp; rm -rf /opt/time/frontend/dist; cp -a /opt/time/releases/$Ts/frontend/dist /opt/time/frontend/dist; chown -R timeapp:timeapp /opt/time/timeapp /opt/time/frontend/dist; systemctl restart timeapp; systemctl is-active timeapp"

# 5) 验证
ssh -i $Key $Server "curl -I --max-time 8 http://127.0.0.1:5174/"
curl.exe -I --max-time 10 http://47.242.41.252:5174/
```

---

## 7. 回滚

如果某次发布异常，可回滚到上一版本（示例）：

```bash
TS=上一版本时间戳
install -m 755 /opt/time/releases/$TS/timeapp /opt/time/timeapp
rm -rf /opt/time/frontend/dist
cp -a /opt/time/releases/$TS/frontend/dist /opt/time/frontend/dist
chown -R timeapp:timeapp /opt/time/timeapp /opt/time/frontend/dist
systemctl restart timeapp
```

---

## 8. 常用排障命令

```bash
systemctl status timeapp --no-pager
journalctl -u timeapp -n 200 --no-pager
ss -lntp | grep 5174
curl -I http://127.0.0.1:5174/
```

如果启动失败，优先检查：
- `/opt/time/.env` 是否存在且 `DB_DSN` 可连通
- 端口是否被占用
- `frontend/dist/index.html` 是否存在
