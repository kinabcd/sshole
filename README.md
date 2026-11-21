# sshole

English | [繁體中文](README-zh.md)

A small, self-contained SSH server written in Go, built mainly for getting a
shell on Windows machines. Copy one binary over, run it, and connect with
any standard SSH client.

- Single static binary, no dependencies on the target machine
- Interactive shell with a real PTY (`cmd.exe` on Windows, `bash`/`sh` on Linux)
- One-shot command execution (`ssh host <command>`) with exit status
- SFTP subsystem, so `scp`/`sftp`/`sshfs` work
- Public-key and password authentication
- Installs itself as a Windows service

## Install

Build it with Go 1.27 or later:

```sh
go install github.com/kinabcd/sshole/cmd/sshole@latest
```

Or from a checkout:

```sh
go tool make          # builds out/sshole.exe (Windows, default)
go tool make linux    # builds out/sshole (Linux)
go tool make clean    # removes out/
```

## Quick start

Run it once in a console so you can see the generated password:

```console
> sshole.exe
sshole: config directory: C:\ProgramData\sshole
sshole: generated initial password (won't be shown again): Xy3...
sshole: 0 authorized public key(s) loaded
sshole: listening addr=[::]:22007
```

Then connect from another machine:

```sh
ssh -p 22007 anyuser@<host>
```

The username is ignored; you get a session running as whatever account
`sshole` itself runs as.

## Running as a Windows service

From an elevated (Administrator) prompt:

```console
> sshole.exe svc install
> sshole.exe svc start
```

The service is set to start automatically on boot. To remove it:

```console
> sshole.exe svc stop
> sshole.exe svc uninstall
```

`svc install` registers the binary at its current path, so put it somewhere
permanent first. Any `-dir` and `-listen` flags given before `svc install` are
saved into the service's command line, e.g.
`sshole.exe -dir D:\sshole -listen :2222 svc install`.

A service runs as `LocalSystem` by default, which means **SSH sessions get
SYSTEM privileges**. Since a service has no console, the initial password
can't be read from its output: set your own with `sshole.exe password <password>`
before starting the service, or use public-key authentication.

On Linux, `svc` subcommands are not supported; use systemd or your own
service manager to run `sshole`.

## Configuration

All state lives in one directory, created on first run:

| OS      | Path                     |
|---------|--------------------------|
| Windows | `%ProgramData%\sshole`   |
| Linux   | `/etc/sshole`            |

To use a different directory, pass `-dir <path>` or set `SSHOLE_CONFIG_DIR`.
`-dir` takes precedence over the environment variable.

| File               | Purpose                                                    |
|--------------------|------------------------------------------------------------|
| `config.json`      | Listen address and bcrypt password hash                    |
| `host_ed25519_key` | Server host key, generated on first run                    |
| `authorized_keys`  | Client public keys allowed to log in, OpenSSH format       |

`config.json` looks like this:

```json
{
  "listen": ":22007",
  "passwordHash": "$2a$10$..."
}
```

- **Change the port:** edit `listen`, or pass `--listen host:port` on the
  command line to override it for one run.
- **Use keys only:** add your public keys to `authorized_keys`, then delete
  the `passwordHash` field to turn off password login.
- **Set or change the password:** run `sshole password <new-password>`
  (add `-dir <path>` if you use a custom directory). Other settings are kept.
  The password ends up in your shell history, so clear it afterwards if that
  matters.

Restart `sshole` after changing any of these files.

## Usage examples

```sh
# Interactive shell
ssh -p 22007 user@host

# One-shot commands (run through cmd.exe /C on Windows)
ssh -p 22007 user@host dir C:\\
ssh -p 22007 user@host powershell -Command "Get-Process"

# File transfer
scp -P 22007 build.zip user@host:C:/temp/
sftp -P 22007 user@host
```

## Limitations

- Port forwarding, agent forwarding and X11 forwarding are not supported.
- `env` requests from the client are ignored.
- SFTP has full filesystem access, with no chroot.
- All authenticated users run as the same OS account.

## Security

`sshole` gives anyone with valid credentials a shell with the server's
privileges. Run it only on networks you trust, prefer public-key
authentication, and keep the config directory readable only by
administrators.

## License

[MIT](LICENSE)
