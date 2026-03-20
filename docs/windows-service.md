# Windows Service Installation (NSSM)

The bot runs as a Windows Service via NSSM (Non-Sucking Service Manager). NSSM wraps the Go binary and handles automatic start at boot, log file redirection, environment variable injection, and service restart on failure.

## Prerequisites

1. **Build the bot binary:**

   ```cmd
   go build -o claude-telegram-bot.exe .
   ```

2. **Download NSSM** from <https://nssm.cc/download> (2.24 stable). Extract `nssm.exe` to a permanent location (e.g., `C:\tools\nssm.exe`). Add it to your PATH or use the full path in commands below.

3. **Locate tool paths:**
   - Claude CLI: typically `C:\Users\<user>\AppData\Roaming\npm\claude.cmd`
   - pdftotext (optional): download Poppler for Windows, e.g., `C:\poppler\Library\bin\pdftotext.exe`

## Install Service

Run all commands in an **Administrator Command Prompt**.

### 1. Install the service

```cmd
nssm install ClaudeTelegramBot "C:\path\to\claude-telegram-bot.exe"
```

### 2. Set working directory

```cmd
nssm set ClaudeTelegramBot AppDirectory "C:\path\to\bot"
```

### 3. Set environment variables

Use `AppEnvironmentExtra` to add variables to the inherited service environment. Do **not** use `AppEnvironment`, which replaces the entire environment.

```cmd
nssm set ClaudeTelegramBot AppEnvironmentExtra ^
  TELEGRAM_BOT_TOKEN=your-bot-token ^
  TELEGRAM_ALLOWED_USERS=123456789 ^
  CLAUDE_CLI_PATH=C:\Users\you\AppData\Roaming\npm\claude.cmd ^
  OPENAI_API_KEY=sk-... ^
  ALLOWED_PATHS=C:\projects ^
  DATA_DIR=C:\bot-data ^
  PDFTOTEXT_PATH=C:\poppler\Library\bin\pdftotext.exe
```

> **Important:** For values containing spaces, use the NSSM GUI instead:
>
> ```cmd
> nssm edit ClaudeTelegramBot
> ```
>
> Navigate to the **Environment** tab and add one variable per line in `KEY=VALUE` format.

### 4. Configure log files

```cmd
nssm set ClaudeTelegramBot AppStdout "C:\logs\claude-bot.log"
nssm set ClaudeTelegramBot AppStderr "C:\logs\claude-bot.err"
nssm set ClaudeTelegramBot AppStdoutCreationDisposition 4
nssm set ClaudeTelegramBot AppStderrCreationDisposition 4
```

`CreationDisposition 4` means append mode -- logs are not overwritten on service restart.

Create the log directory first:

```cmd
mkdir C:\logs
```

### 5. Configure restart on failure

```cmd
nssm set ClaudeTelegramBot AppRestartDelay 5000
```

The service will wait 5 seconds before restarting after a crash.

### 6. Start the service

```cmd
nssm start ClaudeTelegramBot
```

## Manage Service

| Action  | Command |
|---------|---------|
| Status  | `nssm status ClaudeTelegramBot` |
| Stop    | `nssm stop ClaudeTelegramBot` |
| Restart | `nssm restart ClaudeTelegramBot` |
| Edit    | `nssm edit ClaudeTelegramBot` |
| Remove  | `nssm stop ClaudeTelegramBot && nssm remove ClaudeTelegramBot confirm` |

## Environment Variables Reference

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `TELEGRAM_BOT_TOKEN` | Yes | -- | Bot token from BotFather |
| `TELEGRAM_ALLOWED_USERS` | Yes | -- | Comma-separated Telegram user IDs |
| `CLAUDE_CLI_PATH` | No | Auto-detected via PATH | Full path to the `claude` CLI binary |
| `CLAUDE_WORKING_DIR` | No | User home directory | Default working directory for Claude sessions |
| `ALLOWED_PATHS` | No | `CLAUDE_WORKING_DIR` | Comma-separated directories Claude can access |
| `OPENAI_API_KEY` | No | -- | OpenAI API key for voice transcription (Whisper) |
| `PDFTOTEXT_PATH` | No | -- | Full path to `pdftotext` binary for PDF extraction |
| `DATA_DIR` | No | `./data` | Directory for runtime JSON files (mappings, session history) |
| `AUDIT_LOG_PATH` | No | `%TEMP%\claude-telegram-audit.log` | Path to the append-only audit log file |
| `RATE_LIMIT_ENABLED` | No | `true` | Enable per-channel rate limiting |
| `RATE_LIMIT_REQUESTS` | No | `20` | Number of requests allowed per rate limit window |
| `RATE_LIMIT_WINDOW` | No | `60` | Rate limit window duration in seconds |

## Troubleshooting

### Service won't start

1. Check the error log:

   ```cmd
   type C:\logs\claude-bot.err
   ```

2. Verify environment variables are set correctly:

   ```cmd
   nssm dump ClaudeTelegramBot
   ```

3. Test the binary manually first:

   ```cmd
   set TELEGRAM_BOT_TOKEN=your-token
   set TELEGRAM_ALLOWED_USERS=123456789
   C:\path\to\claude-telegram-bot.exe
   ```

### Claude CLI not found

NSSM services may not inherit your user PATH. Set `CLAUDE_CLI_PATH` explicitly to the full path:

```cmd
nssm set ClaudeTelegramBot AppEnvironmentExtra CLAUDE_CLI_PATH=C:\Users\you\AppData\Roaming\npm\claude.cmd
```

Verify the path exists:

```cmd
where claude
```

### Temp directory issues

NSSM services run as the `SYSTEM` account by default, which uses `C:\Windows\Temp` instead of your user temp directory. If you encounter file-not-found errors with voice or document processing, configure the service to run as your user account:

```cmd
nssm set ClaudeTelegramBot ObjectName "DOMAIN\username" "password"
```

Or use the NSSM GUI (**Log on** tab) to set the account.

### PDF extraction not working

1. Verify pdftotext is installed and accessible:

   ```cmd
   C:\poppler\Library\bin\pdftotext.exe -v
   ```

2. Set the `PDFTOTEXT_PATH` environment variable to the full path:

   ```cmd
   nssm set ClaudeTelegramBot AppEnvironmentExtra PDFTOTEXT_PATH=C:\poppler\Library\bin\pdftotext.exe
   ```

3. Restart the service:

   ```cmd
   nssm restart ClaudeTelegramBot
   ```

### Viewing real-time logs

Use PowerShell to tail the log file:

```powershell
Get-Content C:\logs\claude-bot.log -Wait -Tail 50
```
