# WireGo

A simple P2P file sharing CLI application. Share files directly between computers on the same network using a simple 5-digit code.

## Quick Install

**macOS/Linux:**
```bash
curl -sSL https://raw.githubusercontent.com/HappyRish01/wirego/main/install.sh | bash
```

**Windows (PowerShell):**
```powershell
irm https://raw.githubusercontent.com/HappyRish01/wirego/main/install.ps1 | iex
```

## Manual Installation

### Download Binary

Download the latest release for your platform from [Releases](https://github.com/HappyRish01/wirego/releases).

#### Windows
1. Download `wirego_windows_amd64.zip`
2. Extract `wirego.exe`
3. Move to a folder in your PATH (e.g., `C:\Users\YourName\bin`)
4. Or add the folder to your PATH:
   ```powershell
   # Add to PATH permanently (run as admin)
   [Environment]::SetEnvironmentVariable("Path", $env:Path + ";C:\Users\YourName\bin", "User")
   ```

#### macOS
```bash
# Intel Mac
curl -LO https://github.com/HappyRish01/wirego/releases/latest/download/wirego_darwin_amd64.tar.gz
tar -xzf wirego_darwin_amd64.tar.gz
sudo mv wirego /usr/local/bin/

# Apple Silicon (M1/M2/M3)
curl -LO https://github.com/HappyRish01/wirego/releases/latest/download/wirego_darwin_arm64.tar.gz
tar -xzf wirego_darwin_arm64.tar.gz
sudo mv wirego /usr/local/bin/
```

#### Linux
```bash
# x86_64
curl -LO https://github.com/HappyRish01/wirego/releases/latest/download/wirego_linux_amd64.tar.gz
tar -xzf wirego_linux_amd64.tar.gz
sudo mv wirego /usr/local/bin/

# ARM64
curl -LO https://github.com/HappyRish01/wirego/releases/latest/download/wirego_linux_arm64.tar.gz
tar -xzf wirego_linux_arm64.tar.gz
sudo mv wirego /usr/local/bin/
```

### Build from Source
```bash
git clone https://github.com/HappyRish01/wirego.git
cd wirego
go build -o wirego .
```

## Usage

### Send Files

```bash
# Send a single file
wirego send myfile.txt

# Send a directory
wirego send ./myfolder

# Send current directory
wirego send .
```

Output:
```
WireGo - P2P File Sharing
----------------------------------------
Code: 10547
IP: 192.168.1.105
Port: 8127
----------------------------------------

Waiting for the receiver...
Receiver should run: wirego receive 10547 <folder-name>

Press Ctrl+C to cancel
```

### Receive Files

```bash
# Receive files using the code
wirego receive 10547 downloads
```

The files will be saved to the `downloads` folder in your current directory.

## How It Works

1. **Sender** starts sharing and gets a 5-digit code
2. **Receiver** enters the code to connect directly
3. Files are transferred over HTTP on the local network
4. No internet required - works completely offline

### Code Format
The 5-digit code encodes the sender's IP and port:
- First 3 digits: Last octet of sender's IP
- Last 2 digits: Port offset (port = 8080 + offset)

Example: Code `10547` = IP ending in `.105`, Port `8127`

## Requirements

- Both devices must be on the same local network
- No firewall blocking the port (8080-8179)

## License

MIT
