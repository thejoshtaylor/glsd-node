/**
 * Text message handler for Claude Telegram Bot.
 */

import type { Context } from "grammy";
import { session } from "../session";
import { ALLOWED_USERS } from "../config";
import { isAuthorized, rateLimiter } from "../security";
import {
  auditLog,
  auditLogRateLimit,
  checkInterrupt,
  startTypingIndicator,
} from "../utils";
import { StreamingState, createStatusCallback } from "./streaming";
import { autoDocument, formatDocReply } from "../autodoc";
import {
  escapeHtml,
  extractGsdCommands,
  extractNumberedOptions,
  buildActionKeyboard,
} from "../formatting";
import { getLastActionBar, setLastActionBar, getPendingNewProject, setPendingNewProject } from "./commands";
import { addProject } from "../registry";

/**
 * Handle incoming text messages.
 */
export async function handleText(ctx: Context): Promise<void> {
  const userId = ctx.from?.id;
  const username = ctx.from?.username || "unknown";
  const chatId = ctx.chat?.id;
  let message = ctx.message?.text;

  if (!userId || !message || !chatId) {
    return;
  }

  // 1. Authorization check
  if (!isAuthorized(userId, ALLOWED_USERS)) {
    await ctx.reply("Unauthorized. Contact the bot owner for access.");
    return;
  }

  // 1b. Check for pending new project creation
  const pendingProject = getPendingNewProject();
  if (pendingProject && pendingProject.chatId === chatId) {
    setPendingNewProject(null);
    const projectName = message.trim();

    if (!projectName) {
      await ctx.reply("Project creation cancelled (empty name).");
      return;
    }

    try {
      const fullPath = addProject(pendingProject.parentPath, projectName);

      // Kill active session and switch
      if (session.isRunning) {
        await session.stop();
        await new Promise((r) => setTimeout(r, 100));
        session.clearStopRequested();
      }
      if (session.isActive) {
        await session.kill();
      }

      session.setWorkingDir(fullPath.replace(/\//g, "\\"));

      await ctx.reply(
        `📂 Created and switched to <b>${projectName}</b>\n<code>${fullPath}</code>`,
        { parse_mode: "HTML" }
      );
    } catch (error) {
      await ctx.reply(`Failed to create project: ${String(error).slice(0, 200)}`);
    }
    return;
  }

  // 2. Check for interrupt prefix
  message = await checkInterrupt(message);
  if (!message.trim()) {
    return;
  }

  // 3. Rate limit check
  const [allowed, retryAfter] = rateLimiter.check(userId);
  if (!allowed) {
    await auditLogRateLimit(userId, username, retryAfter!);
    await ctx.reply(
      `⏳ Rate limited. Please wait ${retryAfter!.toFixed(1)} seconds.`
    );
    return;
  }

  // 3b. Queue if another query is running
  if (session.isRunning) {
    const queued = session.queueMessage({ ctx });
    if (queued) {
      await ctx.reply("Queued — will process after current request.", { disable_notification: true });
    } else {
      await ctx.reply("Queue full. Please wait for the current request to finish.");
    }
    return;
  }

  // 4. Store message for retry
  session.lastMessage = message;

  // 5. Set conversation title from first message (if new session)
  if (!session.isActive) {
    // Truncate title to ~50 chars
    const title =
      message.length > 50 ? message.slice(0, 47) + "..." : message;
    session.conversationTitle = title;
  }

  // 6. Mark processing started
  const stopProcessing = session.startProcessing();

  // 7. Start typing indicator
  const typing = startTypingIndicator(ctx);

  // 7b. Send "Processing..." message while Claude works
  const processingMsg = await ctx.reply("Processing...", { disable_notification: true });

  // 8. Create streaming state and callback
  let state = new StreamingState();
  let statusCallback = createStatusCallback(ctx, state);

  // 9. Send to Claude with retry logic for crashes
  const MAX_RETRIES = 1;

  for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
    try {
      const response = await session.sendMessageStreaming(
        message,
        username,
        userId,
        statusCallback,
        chatId,
        ctx
      );

      // 10. Audit log
      await auditLog(userId, username, "TEXT", message, response);

      // 10b. Delete processing message before autodoc + context bar
      try {
        await ctx.api.deleteMessage(chatId, processingMsg.message_id);
      } catch { /* already deleted */ }

      // 10c. Auto-document the response (skip for trivial/system responses)
      const isAskUser = response.includes("[Waiting for user selection]");
      const isContextLimit = response.includes("Context limit reached");
      if (!isAskUser && !isContextLimit) {
        try {
          const docResult = await autoDocument(message, response);
          if (docResult) {
            await ctx.reply(formatDocReply(docResult, escapeHtml), {
              parse_mode: 'HTML',
              disable_notification: true,
            });
          }
        } catch (err) {
          console.error("Auto-documentation failed:", err);
        }
      }

      // 10d. Show context bar + action buttons
      {
        const pct = session.contextPercent;
        const barText = pct !== null
          ? (() => {
              const clamped = Math.min(pct, 100);
              const filled = Math.min(Math.round(clamped / 10), 10);
              return "█".repeat(filled) + "░".repeat(10 - filled) + ` ${clamped}%`;
            })()
          : null;

        // Extract GSD suggestions and numbered options from response
        const { commands: gsdCmds, hasClearSuggestion } =
          extractGsdCommands(response);
        const numberedOpts = extractNumberedOptions(response);
        const keyboard = buildActionKeyboard({
          gsdCommands: gsdCmds,
          hasClearSuggestion,
          numberedOptions: numberedOpts,
        });

        // Delete old action bar
        const oldBar = getLastActionBar();
        if (oldBar) {
          try {
            await ctx.api.deleteMessage(oldBar.chatId, oldBar.messageId);
          } catch {}
        }

        const barMsg = await ctx.reply(barText || "—", {
          reply_markup: keyboard,
          disable_notification: true,
        });
        setLastActionBar(chatId, barMsg.message_id);
      }

      break; // Success - exit retry loop
    } catch (error) {
      const errorStr = String(error);
      const isClaudeCodeCrash = errorStr.includes("exited with code");

      // Clean up any partial messages from this attempt
      for (const toolMsg of state.toolMessages) {
        try {
          await ctx.api.deleteMessage(toolMsg.chat.id, toolMsg.message_id);
        } catch {
          // Ignore cleanup errors
        }
      }

      // Retry on Claude Code crash (not user cancellation)
      if (isClaudeCodeCrash && attempt < MAX_RETRIES) {
        console.log(
          `Claude Code crashed, retrying (attempt ${attempt + 2}/${MAX_RETRIES + 1})...`
        );
        await session.kill(); // Clear corrupted session
        await ctx.reply(`⚠️ Claude crashed, retrying...`);
        // Reset state for retry
        state = new StreamingState();
        statusCallback = createStatusCallback(ctx, state);
        continue;
      }

      // Final attempt failed or non-retryable error
      console.error("Error processing message:", error);

      // Delete processing message before sending error
      try {
        await ctx.api.deleteMessage(chatId, processingMsg.message_id);
      } catch { /* already deleted */ }

      // Check if it was a cancellation
      if (errorStr.includes("abort") || errorStr.includes("cancel")) {
        // Only show "Query stopped" if it was an explicit stop, not an interrupt from a new message
        const wasInterrupt = session.consumeInterruptFlag();
        if (!wasInterrupt) {
          await ctx.reply("🛑 Query stopped.");
        }
      } else {
        await ctx.reply("Something went wrong. Try again or /new for a fresh session.");
      }
      break; // Exit loop after handling error
    }
  }

  // 11. Cleanup
  stopProcessing();
  typing.stop();

  // 12. Process next queued message (FIFO)
  const next = session.dequeueMessage();
  if (next) {
    await handleText(next.ctx);
  }
}
