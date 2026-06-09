<p align="right"><a href="README.md">日本語</a> | <b>English</b></p>

# MarkN Resonite Headless Controller (MRHC)

![MRHC web UI](docs/images/screenshot.jpg)

A tool to operate and manage a Resonite headless server from a browser on your LAN.

**Supported platforms:** Windows (x64) / Linux (x64) / Linux (ARM)

It runs as a single binary with no runtime required, and MRHC downloads Resonite itself into its own folder automatically.

## Before you start

You'll need the following to use MRHC:

- **A Resonite headless code (required)** — You must support Resonite via a [subscription](https://account.resonite.com/) (Stripe) at the $10/month tier or higher, and have obtained the headless code (→ [how to get it](#headless-code)).
- **A separate Steam account** — MRHC uses Steam to download/update Resonite. Because MRHC stores this password and does not support two-factor authentication, create a **dedicated sub-account** (separate from your everyday one) and make sure its **Steam Guard (two-factor authentication) is turned off** (→ [how](#steam-guard-off)).
- **Resonite is downloaded automatically** — MRHC fetches Resonite for you, so there's no need to download it beforehand. You don't need to install the Steam client either.
- **Pick a location with plenty of free space** — Resonite itself and its cache use a fair amount of space (MRHC can delete old cache automatically).
- **Decide the location first** — Resonite and your settings are stored inside the folder, and their absolute paths are recorded. **Do not move or rename the folder after the first launch (especially after Resonite has been downloaded).** Put it where you want it before starting.
- **Access is from the LAN only** — The web UI (admin screen) is reachable **only from the same network**. To use it while away, you'll need a VPN or something like Tailscale (→ [running on a VPS](#install-vps)).

## Installation

Choose your environment: [Windows](#install-windows) / [Linux (x64 / ARM)](#install-linux) / [VPS (Oracle Cloud, etc.)](#install-vps)

<a id="install-windows"></a>
### Windows

1. Download `mrhc-windows-amd64.zip` from the [releases page](https://github.com/MarkN2000/MarkNResoniteHeadlessController/releases/latest).
2. **Extract it where you intend to use it**, then run `mrhc.exe` inside the folder (don't move the folder after launching).
3. On first launch, a setup wizard (Japanese/English) starts. After you set the admin password and port, Resonite begins downloading. **The download takes a while — wait until you see "Start the server now?".**

Once it's running, open `http://localhost:8080` (default) in your browser to use the web UI.

> **Closing the window stops MRHC too.** Closing the console window where you ran `mrhc.exe` (or signing out / shutting down the PC) stops MRHC. Keep the window open while it's running. If you want it to keep running after sign-out or to start at boot, you can register it with Windows "Task Scheduler" to launch at logon, for example.
>
> **About the SmartScreen warning** — Because the binary is unsigned, you may see "Windows protected your PC" on first run. Click "More info" → "Run anyway" to start it.
>
> **Where data is stored** — Settings, state, and the downloaded Resonite are all kept inside the same folder as `mrhc.exe`. To back up, copy the whole folder (as noted above, avoid moving/renaming it after launch).

<a id="install-linux"></a>
### Linux (x64 / ARM)

Move to where you want it, then run this single line. The command is the same for x64 and ARM (the architecture is detected automatically).

```sh
cd ~   # wherever you want it (e.g. your home directory — it always exists)
curl -fsSL https://github.com/MarkN2000/MarkNResoniteHeadlessController/releases/latest/download/install.sh | sh
```

This creates `mrhc-linux-amd64/` (or `mrhc-linux-arm64/` on ARM) where you ran it, so start it from inside that folder.

```sh
cd mrhc-linux-amd64   # mrhc-linux-arm64 on ARM
./mrhc
```

On first launch, a setup wizard (Japanese/English) starts. After you set the admin password and port, Resonite begins downloading (it takes a while — wait until you see "Start the server now?"). When it finishes, the server comes up and you can use the web UI at `http://<server IP>:8080` (default).

> **Closing the terminal (window) stops MRHC too.** Especially when operating over SSH, if you just launch `./mrhc`, the whole process stops the moment you disconnect SSH. To keep it running after you close the window, see "**Keep the process running**" (tmux / systemd) in the [VPS (Oracle Cloud, etc.)](#install-vps) section — the same steps work even if you're not on a VPS.

The .NET runtime and DepotDownloader are fetched automatically by MRHC, so aside from the firewall there's basically nothing to prepare in advance (if a dependency such as freetype2 is missing, you'll be guided to install it during setup).

> **Where data is stored** — Settings, state, and Resonite are all kept inside the same folder as `mrhc` (use `-data <dir>` to put them elsewhere). Don't move or rename the folder after launching. To extract manually, unpack `mrhc-linux-amd64.tar.gz` / `mrhc-linux-arm64.tar.gz` from the [latest release](https://github.com/MarkN2000/MarkNResoniteHeadlessController/releases/latest) anywhere you like (no `chmod +x` needed — the executable bit is preserved).

**Firewall (reference)** — If other users on the same LAN can't join or find your session, allow inbound UDP from the LAN (adjust `192.168.1.0/24` to your LAN's address range).

- ufw (Ubuntu-based, CachyOS, etc.): `sudo ufw allow from 192.168.1.0/24 proto udp`
- firewalld (Fedora, RHEL-based, openSUSE, etc.): `sudo firewall-cmd --permanent --add-rich-rule='rule family="ipv4" source address="192.168.1.0/24" protocol value="udp" accept' && sudo firewall-cmd --reload`
- On systems where the firewall is disabled by default (e.g. vanilla Arch Linux), no configuration is needed.

<a id="install-vps"></a>
### VPS (Oracle Cloud, etc.)

> ⚠️ **Important: the web UI can only be opened from the LAN.** When running on a VPS, you can't reach it directly from the internet. Create a "same network" state with an **SSH tunnel** or a **VPN such as Tailscale** before accessing it. Because the web UI is plain HTTP, never expose its port directly to the internet (the admin password would travel in plaintext).

**Recommended setup (this section assumes it)**

- An Oracle Cloud **Ampere A1 (ARM)** instance
- OS: **Ubuntu**

**1. Install** (same as Linux ARM) — Log in over SSH and run the following. **If you have no particular preference, just run it where you land after logging in (your home directory)** (it extracts into the current directory, so only `cd` somewhere first if you want it elsewhere).

```sh
curl -fsSL https://github.com/MarkN2000/MarkNResoniteHeadlessController/releases/latest/download/install.sh | sh
```

> **Don't use `sudo`.** Extracting as root makes the folder owned by root, so MRHC's self-update can't write next to the executable and will fail. Run it as your login user (`ubuntu` on Oracle's Ubuntu).

**2. Start it (inside tmux, so it keeps running after you close SSH)**

If you just run `./mrhc`, **MRHC stops the moment you close SSH (close the terminal).** Starting it inside `tmux` keeps it running even after you disconnect. The first-run setup wizard works interactively inside tmux too, so it's handy for the initial launch as well.

```sh
sudo apt install -y tmux       # if not already installed
tmux new -s mrhc               # create a session named mrhc
cd mrhc-linux-arm64 && ./mrhc  # start it here (the wizard runs on first launch)
```

Once it's up, press **`Ctrl + B`, then `D`** to "detach" from the session. MRHC keeps running inside tmux even after you close SSH. To see the logs again, run `tmux attach -t mrhc` (if you prefer `screen`, `screen -S mrhc` works the same way).

> tmux is easy, but **it does not survive a reboot of the server (VM) itself.** If you want MRHC to start automatically after the OS reboots, or to recover automatically when it crashes, use systemd below.

**(+α, optional) Start automatically after a reboot — run it as a systemd service**

For long-term operation, registering it with systemd makes MRHC start automatically after a VM reboot and recover automatically if it goes down. **First complete the setup wizard once via tmux** (etc.) above so `mrhc.config.json` is created, stop that MRHC inside tmux with `Ctrl + C`, and then configure the following.

Create `/etc/systemd/system/mrhc.service` (adjust the paths and user name to your environment):

```ini
[Unit]
Description=MarkN Resonite Headless Controller
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=ubuntu
WorkingDirectory=/home/ubuntu/mrhc-linux-arm64
ExecStart=/home/ubuntu/mrhc-linux-arm64/mrhc
Restart=on-failure
RestartSec=5
TimeoutStopSec=200

[Install]
WantedBy=multi-user.target
```

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now mrhc   # start now + start automatically on next boot
journalctl -u mrhc -f              # view the logs
```

- **Don't omit `TimeoutStopSec=200`.** MRHC waits up to 185 seconds to close worlds safely when stopping. If this is too short, it gets force-killed mid-stop and the Resonite headless can be left orphaned.
- **Don't run it as root.** Because self-update (web UI / `mrhc update`) writes next to the executable, set `User=` to the owner of the mrhc folder (`ubuntu` in this example).
- The web UI's "Restart now" (e.g. after applying an update) works under systemd too (it replaces the same process, so systemd doesn't treat it as an exit).

**3. Access the web UI** (use either method to create a "same network" state)

**Option A: SSH tunnel** (no extra install, no port opening needed)

On your local PC, run (from a phone, an SSH app such as Termius works too):

```sh
ssh -L 8080:localhost:8080 <user>@<VM IP>
```

Keep it connected and open `http://localhost:8080` in your local browser. Since SSH (port 22) is already getting through, the firewall is already open for it, so no additional opening is needed.

**Option B: Tailscale** (VPN; convenient from a phone too)

Putting the VPS and your local device on the same Tailscale network (tailnet) makes them act as one network, so you can access the web UI directly.

1. Install Tailscale on the VPS and connect.

   ```sh
   curl -fsSL https://tailscale.com/install.sh | sh
   sudo tailscale up
   ```

   Open the URL it shows to log in and authorize.
2. Install [Tailscale](https://tailscale.com/download) on your local PC/phone too and log in with the same account.
3. Check the VPS's Tailscale IP (run `tailscale ip -4` on the VPS) and open `http://<that IP>:8080` in your local browser.

**4. (Optional) Speed up direct session joins — open a UDP port**

Even without opening a port, others can join your session through Resonite's relay (with extra latency). Only set this up if you want lower latency via a direct connection. A cloud VM's firewall has **two layers** (the cloud-side security rule + the VM's own), and both must be opened.

1. **Fix the port number** — In the web UI "Config" tab, open the world, add `forcePort` from "Advanced fields" with any number (e.g. `32100`), save, and restart the headless (the default is a random port, so fixing it is required).
2. **Open the cloud side** — Oracle Cloud console → the relevant VCN's "Security List" (or NSG) → add an ingress rule (source `0.0.0.0/0`, protocol `UDP`, destination port `32100`).
3. **Open the VM side (Ubuntu)** — Oracle's Ubuntu uses raw iptables (ufw is disabled by default).

   ```sh
   sudo iptables -I INPUT -p udp --dport 32100 -j ACCEPT
   sudo netfilter-persistent save
   ```

   `-I INPUT` **inserts the rule at the top** (so it sits before Oracle's default trailing REJECT; with `-A` (append) it would be rejected and have no effect).

> Anyone will be able to join the session, but access control is handled by Resonite's accessLevel, not the firewall.

## Updating

MRHC keeps itself up to date with its built-in self-update.

- **From the web UI** — When a new version is available, a red dot appears on the ⋮ at the top right. ⋮ → "Check for updates" → "Update" downloads (with a progress bar), verifies, and swaps it in automatically, and you're on the new version **from the next time you restart MRHC** (the swap itself doesn't affect running worlds). Then press "Restart now" to stop the worlds one by one, after which MRHC relaunches itself on the new version and the screen reconnects automatically (this can take a few minutes if stopping is slow).
- **From the command line** — `./mrhc update` (Windows: `mrhc.exe update`). This also serves as a recovery method when MRHC won't start.

> To update manually, stop MRHC and re-run install.sh (Linux), or extract the zip over the same location (Windows). Settings and data aren't included in the archive, so they're preserved either way.

<a id="headless-code"></a>
## How to get a headless code

Resonite's headless server is distributed as a **private beta**, and a "headless code" is required to download and run it.

1. Support Resonite via a [subscription](https://account.resonite.com/) (Stripe — lower fees, officially recommended) and reach a tier that includes headless ($10/month or higher).
2. Launch Resonite and send **`/headlessCode`** to the **Resonite bot** in your friends list.
3. Enter the returned code into MRHC's setup wizard (or the branch code field under "Settings → Steam").

> The code can change. If things stop working after an update, get the latest code again with `/headlessCode` and re-enter it.

## Troubleshooting

- **I want to see the headless logs** — In the web UI "Logs" tab, you can pick a Resonite headless log file (`<install dir>/Headless/Logs`) to view and copy (read-only; viewable even while stopped). Large logs show only the tail. The current log of a running server may be unreadable depending on the OS.
- **I'm worried about disk space (cache)** — Under "Cache management" in the Settings tab, you can check the total size of the Resonite cache (default `headless-cache`) and clear it entirely (only while the headless is stopped). Turning on "Auto-delete cache" cleans up, every time it stops, any cache whose last modification is older than a set number of days (default 30). Anything still needed is re-fetched automatically next time.
- **I forgot the admin password** — Run `./mrhc reset-password` (Windows: `mrhc.exe reset-password`) on the server's command line to reset it without the old password.
- **An update failed midway and it won't start** — If `mrhc.exe.old` (Linux: `mrhc.old`) remains next to the executable, rename it back to `mrhc.exe` (`mrhc`) to recover the previous version.
- **I want to redo setup from scratch** — Delete `mrhc.config.json` in the folder and start again; the wizard runs again.
- **I want to change the port / it says the port is in use** — Change `"port"` in `mrhc.config.json` to another number and start again.
- **I can't join/find the session from the same LAN** — On the server, allow inbound UDP from the LAN.
  - **Windows**: Change the connected network to a **Private network** (Settings → Network & internet → Ethernet, or Wi-Fi).
  - **Linux**: If a firewall is enabled, allow UDP from the LAN (see "Firewall (reference)" under [Linux installation](#install-linux)).
- <a id="steam-guard-off"></a>**I can't turn off Steam Guard** — If the account has the mobile authenticator set up, first remove it in the Steam mobile app (Steam Guard → Remove Authenticator), then turn it off under Steam "Settings → Security".
- **I want to change the display language** — Change `"language"` in `mrhc.config.json` to `"ja"` / `"en"` and restart (the web UI's display language is managed separately via the toggle at the top right).

## Key features

Manage your entire headless server from a browser on your LAN.

**Start / stop / monitoring**
- Start / graceful stop (stops safely in about 2 minutes) / force stop / restart of the headless (Resonite process)
- Live status: participants (present/away), world info, uptime, session URL
- Live log of Resonite output (SSE), plus viewing and copying log files
- Server resource usage (CPU / memory / free disk)
- Send arbitrary console commands

**Running sessions**
- Open a new session: from a template, a record URL, or by **searching worlds by keyword (world name & tags)**; save favorites
- Edit session settings: name, access level, max users, description, hide from listing
- Participant actions: kick / ban / mute / respawn / role change / send message
- Spawn items (by URL) and send dynamic impulses (tag + value)

**User management**
- Search by username or user ID; send and remove friend requests
- Accept friend requests; invite to the focused session
- Ban and unban across all sessions

**Config (headless settings)**
- Create, duplicate, rename, and save multiple configs and switch between them
- Per-world settings following the official schema (access level, tags, AFK kick, autosave, auto-recover, roles, auto-invite, forcePort, etc.)
- Add any schema field from "Advanced fields"

**Auto-restart & maintenance**
- Scheduled restarts (once / daily / weekly), with pre-actions such as an announcement (dynamicImpulse), going private, or renaming
- Restart safely after waiting for users to leave
- Automatic recovery from crashes (with a crash-count guard against runaway loops)
- Optionally update Resonite during a scheduled restart
- Cache management (auto-delete on stop, manual full clear, size check)

**Setup & distribution**
- Auto-download/update Resonite via DepotDownloader (all OSes, ARM included; the .NET runtime is installed automatically too)
- Detect missing dependencies (e.g. freetype2) and guide installation
- Self-update of MRHC itself (web UI / CLI)
- Japanese/English support (setup wizard and web UI); a single binary with no runtime required (Windows / Linux x64 / ARM)

## Documentation

- Design doc: [docs/DESIGN.md](docs/DESIGN.md)
- Resonite domain facts (console commands, output formats, how to launch, etc.): [docs/resonite-domain-facts.md](docs/resonite-domain-facts.md)

> These documents are written in Japanese.

## Building / development

Prerequisites: **Go 1.26+** and **Node 20+**.

```sh
# 1) Build the frontend (generates web/dist → embedded by Go)
cd web && npm install && npm run build && cd ..

# 2) Build the binary
go build -o bin/mrhc ./cmd/mrhc                                              # for the current OS
GOOS=windows GOARCH=amd64 go build -o bin/mrhc-windows-amd64.exe ./cmd/mrhc  # Windows (x64)
GOOS=linux   GOARCH=amd64 go build -o bin/mrhc-linux-amd64      ./cmd/mrhc   # Linux (x64)
GOOS=linux   GOARCH=arm64 go build -o bin/mrhc-linux-arm64      ./cmd/mrhc   # Linux (ARM64)
```

All targets are **pure Go with no CGO** (dependencies are only `golang.org/x/{crypto,sys,term,text}`), so you can cross-compile just by changing the environment variables. The full multi-target release build is done by GitHub Actions (`.github/workflows/release.yml`).

> ⚠️ `web/dist` is a build artifact and is **not tracked by git**. Always build the frontend before `go build` (it's bundled via embed.FS).

During development:

- Backend: `go run ./cmd/mrhc -data ./bin/devdata` (interactive setup on first run)
- Frontend: `cd web && npm run dev` (proxies `/api` to the backend on `:8080`)

## License

MIT — [LICENSE](LICENSE)

This software is a tool for operating a Resonite headless server. When using it, please follow Resonite's [guidelines](https://resonite.com/policies/Guidelines.html) and terms of service.
