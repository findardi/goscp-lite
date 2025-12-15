# goscp-lite

A lightweight, high-performance CLI tool for secure file transfers using the SFTP protocol.

## Features

- **Concurrent Uploads**: Maximizes bandwidth usage with parallel workers.
- **Resumable**: Automatically resumes interrupted uploads.
- **Reliable**: Ensures data integrity with atomic renaming and MD5 checksum verification.
- **Secure**: Supports standard SSH key authentication.

## Installation

```bash
go build -o goscp .
```

## Usage

```bash
./goscp upload <local-path> <remote-path> [flags]
```

### Flags

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--host` | `-H` | **(Required)** Remote server host | |
| `--user` | `-u` | SSH Username | `root` |
| `--port` | `-p` | SSH Port | `22` |
| `--key` | `-k` | Path to private SSH key | Auto-detect |

### Examples

**Upload file**
```bash
./goscp upload ./data.zip /var/www/backups/ -H 192.168.1.50 -u admin
```

**Resume upload**
Run the same command again to resume from the last successful point.

**Upload with specific key**
```bash
./goscp upload ./app.tar.gz /home/deploy/ -H example.com -k ~/.ssh/prod_key.pem
```

## License

MIT License
