# sshole

[English](README.md) | 繁體中文

用 Go 寫的輕量 SSH 伺服器，主要用來在 Windows 機器上開一個 shell。只要把一個執行檔複製過去並執行，就能用任何標準 SSH 用戶端連線。

- 單一靜態執行檔，目標機器不需要安裝任何相依套件
- 有真正 PTY 的互動 shell（Windows 用 `cmd.exe`，Linux 用 `bash`/`sh`）
- 單次執行指令（`ssh host <指令>`），並回傳結束代碼
- 支援 SFTP 子系統，可以使用 `scp`、`sftp`、`sshfs`
- 支援公開金鑰和密碼兩種登入方式
- 可以自行安裝成 Windows 服務

## 安裝

從[最新版本](https://github.com/kinabcd/sshole/releases/latest)下載 `sshole-windows-amd64.exe` 或 `sshole-linux-amd64`。每次推送到 `main` 都會自動建置新版本，版號格式為 `yyyymmdd.N`。

或是自行建置，需要 Go 1.27 以上版本：

```sh
go install github.com/kinabcd/sshole/cmd/sshole@latest
```

或是從原始碼建置：

```sh
go tool make          # 建置 out/sshole.exe（Windows，預設）
go tool make linux    # 建置 out/sshole（Linux）
go tool make clean    # 刪除 out/
```

## 快速上手

先在主控台執行一次，才能看到自動產生的密碼：

```console
> sshole.exe
sshole: config directory: C:\ProgramData\sshole
sshole: generated initial password (won't be shown again): Xy3...
sshole: 0 authorized public key(s) loaded
sshole: listening addr=[::]:22007
```

接著從另一台機器連線：

```sh
ssh -p 22007 anyuser@<host>
```

使用者名稱會被忽略，連線後的 session 一律以 `sshole` 本身的執行帳號執行。

## 安裝成 Windows 服務

在系統管理員權限的命令提示字元中執行：

```console
> sshole.exe svc install
> sshole.exe svc start
```

服務會設定為開機時自動啟動。要移除服務：

```console
> sshole.exe svc stop
> sshole.exe svc uninstall
```

`svc install` 會記錄執行檔目前的路徑，所以請先把執行檔放到固定的位置再安裝。寫在 `svc install` 前面的 `-dir` 和 `-listen` 參數會一併存進服務的啟動指令，例如 `sshole.exe -dir D:\sshole -listen :2222 svc install`。

Windows 服務預設以 `LocalSystem` 身分執行，這代表 **SSH 連線會取得 SYSTEM 權限**。服務沒有主控台，看不到初始密碼，所以請在啟動服務前先用 `sshole.exe password <密碼>` 設定自己的密碼，或是改用公開金鑰登入。

Linux 不支援 `svc` 子指令，請用 systemd 或其他服務管理工具來執行 `sshole`。

## 設定

所有資料都放在同一個資料夾，第一次執行時會自動建立：

| 作業系統 | 路徑                   |
|----------|------------------------|
| Windows  | `%ProgramData%\sshole` |
| Linux    | `/etc/sshole`          |

要改用其他資料夾，可以加上 `-dir <路徑>` 參數，或設定 `SSHOLE_CONFIG_DIR` 環境變數。兩者都有設定時，以 `-dir` 為準。

| 檔案               | 用途                                       |
|--------------------|--------------------------------------------|
| `config.json`      | 監聽位址和 bcrypt 密碼雜湊                 |
| `host_ed25519_key` | 伺服器 host key，第一次執行時自動產生      |
| `authorized_keys`  | 允許登入的用戶端公開金鑰，OpenSSH 格式     |

`config.json` 的內容如下：

```json
{
  "listen": ":22007",
  "passwordHash": "$2a$10$..."
}
```

- **更改 port：** 修改 `listen`，或在命令列加上 `--listen host:port`，只覆寫這一次執行的設定。
- **只用金鑰登入：** 把公開金鑰加進 `authorized_keys`，再刪掉 `passwordHash` 欄位，就會關閉密碼登入。
- **設定或更改密碼：** 執行 `sshole password <新密碼>`（如果使用自訂資料夾，要加上 `-dir <路徑>`），其他設定會保留。密碼會留在 shell 的指令紀錄裡，必要時記得清除。

修改上述任何檔案後，都需要重新啟動 `sshole`。

## 使用範例

```sh
# 互動 shell
ssh -p 22007 user@host

# 單次執行指令（Windows 上透過 cmd.exe /C 執行）
ssh -p 22007 user@host dir C:\\
ssh -p 22007 user@host powershell -Command "Get-Process"

# 傳輸檔案
scp -P 22007 build.zip user@host:C:/temp/
sftp -P 22007 user@host
```

## 限制

- 不支援 port forwarding、agent forwarding 和 X11 forwarding。
- 會忽略用戶端送來的 `env` 請求。
- SFTP 可以存取整個檔案系統，沒有 chroot 限制。
- 所有通過驗證的使用者都以同一個作業系統帳號執行。

## 安全性

任何擁有有效憑證的人，都能以伺服器的權限取得 shell。請只在信任的網路上執行 `sshole`，優先使用公開金鑰登入，並確保設定資料夾只有系統管理員能讀取。

## 授權

[MIT](LICENSE)
