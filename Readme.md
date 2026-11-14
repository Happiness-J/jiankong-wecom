# jiankong-wecom

把一台 Linux 服务器的状态做成日报，通过企业微信群机器人推送到群里。单文件 Go 程序、无常驻进程，配合 systemd 定时器每天定时发送一次。

推送内容：主机名、内网 IP、CPU / 内存 / 磁盘使用率（可设阈值，超限显示 ⚠️）、网络流量、系统版本与虚拟化类型、已运行时长。

同系列：钉钉版 jiankong-dingtalk、飞书版 jiankong-feishu。

## 企业微信机器人准备

1. 在目标群里：群设置 → 群机器人 → 添加机器人 → 新创建一个机器人
2. 保存好 Webhook 地址（形如 `https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=xxxxxxxx`）
3. 把整个地址填到 `WECOM_WEBHOOK`（企业微信机器人没有签名机制，key 就在地址里，注意保密）

## 配置（环境变量）

| 变量 | 必填 | 默认 | 说明 |
| --- | --- | --- | --- |
| `WECOM_WEBHOOK` | 是 | - | 企业微信群机器人 Webhook 完整地址 |
| `CUSTOM_TITLE` | 否 | 服务器状态日报 | 日报标题 |
| `CPU_THRESHOLD` | 否 | 80 | CPU 使用率告警阈值（%） |
| `MEM_THRESHOLD` | 否 | 80 | 内存使用率告警阈值（%） |
| `DISK_THRESHOLD` | 否 | 80 | 磁盘使用率告警阈值（%） |
| `REPORT_TIME` | 否 | - | 计划推送时刻，仅在日报中展示，实际调度由 systemd 定时器决定 |

## 使用

### 进项目根目录
```
cd jiankong-wecom
```
### 拉依赖
```
go mod tidy
```
### 编译
```
CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=v1.0.0" -o jiankong-wecom .
```
### 本地运行
```
export WECOM_WEBHOOK='https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx'
./jiankong-wecom
```

### systemd 管理
#### 二进制放置
```
sudo cp jiankong-wecom /usr/local/bin/
sudo chmod +x /usr/local/bin/jiankong-wecom
```

#### systemd 单元文件
```
sudo tee /etc/systemd/system/jiankong-wecom.service >/dev/null <<'EOF'
[Unit]
Description=Push server status to WeCom once
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
Environment=WECOM_WEBHOOK=https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=YOUR_KEY
Environment=CPU_THRESHOLD=85
Environment=MEM_THRESHOLD=85
Environment=DISK_THRESHOLD=85
Environment=REPORT_TIME=09:30
ExecStart=/usr/local/bin/jiankong-wecom
EOF
```
#### systemd 定时器
```
sudo tee /etc/systemd/system/jiankong-wecom.timer >/dev/null <<'EOF'
[Unit]
Description=Daily jiankong to WeCom

[Timer]
OnCalendar=*-*-* 09:30:00
Persistent=true

[Install]
WantedBy=timers.target
EOF
```

### 启用并立即测试
```
sudo systemctl daemon-reload
sudo systemctl enable --now jiankong-wecom.timer
sudo systemctl start jiankong-wecom.service
journalctl -u jiankong-wecom --no-pager -e
```

## 更新记录

### v1.0.0 (2026-11-15)
- 首个版本：服务器状态日报推送到企业微信群（CPU / 内存 / 磁盘 / 网络 / 主机信息）
- 推送失败（企业微信 errcode 非 0）会正确报错退出，不会误报成功
