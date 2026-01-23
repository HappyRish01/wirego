# WireGo

Share files between computers without the cloud. Just you, your friend, and your local network.

## What is this?

Ever wanted to send a file to your friend sitting next to you, but ended up uploading it to Google Drive, waiting, then having them download it? Stupid, right?

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

## The code thing

The 5-digit code isn't random - it actually contains the sender's network info encoded in it. So when your friend enters the code, WireGo knows exactly where to connect. No scanning, no configuration.

## Things to know

- Both computers need to be on the **same WiFi/network**
- Your firewall might block it - if it asks, allow it
- The transfer is **not encrypted** (it's local network, but still - don't send your passwords.txt)
- Works on Windows, Mac, and Linux

## When it doesn't work

**"Connection refused"** - The sender closed the terminal or you typed the wrong code.

**"Connection timed out"** - You're probably not on the same network, or there's a firewall blocking it.

**Can't find local IP** - You might not be connected to any network.

## Build it yourself

```bash
git clone https://github.com/HappyRish01/wirego.git
cd wirego
go build .
```

## Why I made this

I got tired of:
- Uploading to Google Drive just to share with someone 5 feet away
- Typing long `scp` commands and forgetting the syntax every time
- USB drives (who carries those anymore?)

So I built this. It's simple, it works, and it's fast.

## License

MIT - do whatever you want with it.
