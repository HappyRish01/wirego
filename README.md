# WireGo

Share files between computers without the cloud. Just you, your friend, and your local network.

WireGo lets you send files directly from your computer to theirs. No uploads. No accounts. No internet required (just the same WiFi).

```
You (sender)                    Your friend (receiver)
    │                                   │
    │         Local Network             │
    └───────────────────────────────────┘
              Direct transfer
              No cloud needed
```

## Install

**Mac/Linux:**
```bash
curl -sSL https://raw.githubusercontent.com/HappyRish01/wirego/main/install.sh | bash
```

**Windows (open PowerShell):**
```powershell
irm https://raw.githubusercontent.com/HappyRish01/wirego/main/install.ps1 | iex
```

Or just grab the binary from [releases](https://github.com/HappyRish01/wirego/releases) and put it somewhere in your PATH.

## How to use

**Person sending the file:**
```bash
wirego send report.pdf
```

You'll see something like:
```
WireGo - P2P File Sharing
----------------------------------------
Code: 10547
----------------------------------------

Waiting for the receiver...
```

Tell your friend that code.

**Person receiving:**
```bash
wirego receive 10547 downloads
```

That's it. File lands in the `downloads` folder.

### Sending folders

Works the same way:
```bash
wirego send ./my-project
```

WireGo zips it up, sends it, and unzips it on the other end automatically.

### Sending everything in current directory

```bash
wirego send .
```


## Things to know

- Both computers need to be on the **same WiFi/network**
- Your firewall might block it - if it asks, allow it
- The transfer is **not encrypted** (it's local network, but still - don't send your passwords.txt)
- Works on Windows, Mac, and Linux

## Build it yourself

```bash
git clone https://github.com/HappyRish01/wirego.git
cd wirego
go build .
```


## License

MIT - do whatever you want with it.
