/**
 * Auto-documentation pipeline for Claude Code responses.
 *
 * DISABLED by default. Set AUTODOC_ENABLED=true in .env to activate.
 *
 * When enabled, takes a raw Claude Code response + original user query,
 * classifies the content, creates a formatted Obsidian document with YAML
 * frontmatter, optionally updates the category index file, and optionally
 * sends an email notification via Himalaya CLI.
 *
 * Required env vars when enabled:
 *   AUTODOC_ENABLED=true
 *   OBSIDIAN_VAULT_ROOT=/path/to/vault
 *
 * Optional env vars:
 *   AUTODOC_EMAIL_ENABLED=true
 *   AUTODOC_EMAIL_FROM=sender@example.com
 *   AUTODOC_EMAIL_TO=recipient@example.com
 *   HIMALAYA_PATH=/path/to/himalaya
 *   AUTODOC_CATEGORIES_FILE=/path/to/categories.json
 */

import { writeFileSync, readFileSync, mkdirSync, existsSync, unlinkSync } from 'fs';
import { join } from 'path';
import { execFileSync } from 'child_process';
import { tmpdir } from 'os';

// ============== Configuration ==============

const AUTODOC_ENABLED = (process.env.AUTODOC_ENABLED || 'false').toLowerCase() === 'true';
const VAULT_ROOT = process.env.OBSIDIAN_VAULT_ROOT || '';

const EMAIL_ENABLED = AUTODOC_ENABLED && (process.env.AUTODOC_EMAIL_ENABLED || 'false').toLowerCase() === 'true';
const HIMALAYA_PATH = process.env.HIMALAYA_PATH || 'himalaya';
const EMAIL_FROM = process.env.AUTODOC_EMAIL_FROM || '';
const EMAIL_TO = process.env.AUTODOC_EMAIL_TO || '';

// ============== Types ==============

export interface AutoDocResult {
  title: string;
  category: string;
  vaultPath: string;
  tags: string[];
  emailSent: boolean;
  summary: string;
}

/** Format autodoc result as an HTML reply for Telegram */
export function formatDocReply(doc: AutoDocResult, escapeHtml: (s: string) => string): string {
  return [
    `\u{1F4C4} <b>${escapeHtml(doc.title)}</b>`,
    `\u{1F4C2} <code>${escapeHtml(doc.vaultPath)}</code>`,
    doc.emailSent ? '\u{2709}\u{FE0F} Email sent' : '',
  ].filter(Boolean).join('\n');
}

interface CategoryEntry {
  folder: string;
  index: string | null;
  label: string;
}

// ============== Default Category Map ==============

const DEFAULT_CATEGORY_MAP: Record<string, CategoryEntry> = {
  knowledge_base: { folder: 'Knowledge Base/', index: null, label: 'Knowledge Base' },
  ideas:          { folder: 'Ideas/',          index: null, label: 'Ideas' },
  inbox:          { folder: 'Inbox/',          index: null, label: 'Inbox' },
};

// ============== Default Keyword Scoring ==============

const DEFAULT_CATEGORY_KEYWORDS: Record<string, Array<[string, number]>> = {
  knowledge_base: [
    ['programming', 3], ['code', 2], ['typescript', 3], ['javascript', 3], ['python', 3],
    ['algorithm', 2], ['pattern', 2], ['architecture', 3], ['design pattern', 3],
    ['debugging', 3], ['troubleshoot', 3], ['error', 2], ['bug', 2], ['fix', 2],
    ['api', 2], ['database', 3], ['sql', 3], ['how to', 2], ['tutorial', 2],
    ['guide', 2], ['workflow', 2], ['configuration', 2], ['setup', 2],
    ['technical', 2], ['implementation', 2], ['refactor', 2], ['optimize', 2],
  ],
  ideas: [
    ['idea', 3], ['concept', 2], ['brainstorm', 3], ['creative', 2], ['innovation', 2],
    ['project idea', 3], ['what if', 3], ['imagine', 2], ['explore', 2],
  ],
};

// ============== Load Custom Config ==============

let CATEGORY_MAP: Record<string, CategoryEntry> = DEFAULT_CATEGORY_MAP;
let CATEGORY_KEYWORDS: Record<string, Array<[string, number]>> = DEFAULT_CATEGORY_KEYWORDS;

if (process.env.AUTODOC_CATEGORIES_FILE && existsSync(process.env.AUTODOC_CATEGORIES_FILE)) {
  try {
    const custom = JSON.parse(readFileSync(process.env.AUTODOC_CATEGORIES_FILE, 'utf-8'));
    if (custom.categories) CATEGORY_MAP = custom.categories;
    if (custom.keywords) CATEGORY_KEYWORDS = custom.keywords;
    console.log('[autodoc] Loaded custom categories from', process.env.AUTODOC_CATEGORIES_FILE);
  } catch (err) {
    console.error('[autodoc] Failed to load custom categories:', err);
  }
}

// ============== Utility Functions ==============

function slugify(text: string, maxLen = 60): string {
  return text
    .toLowerCase()
    .replace(/[^a-z0-9\s-]/g, '')
    .trim()
    .replace(/\s+/g, '-')
    .slice(0, maxLen)
    .replace(/-$/, '');
}

function todayStr(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

function wordCount(text: string): number {
  return text.trim().split(/\s+/).filter(Boolean).length;
}

// ============== Classification ==============

function classifyContent(query: string, response: string): string {
  const combinedText = (query + ' ' + response).toLowerCase();
  const scores: Record<string, number> = {};

  for (const [category, keywords] of Object.entries(CATEGORY_KEYWORDS)) {
    let score = 0;
    for (const [keyword, weight] of keywords) {
      const escaped = keyword.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
      const regex = new RegExp(`\\b${escaped}\\b`, 'gi');
      const matches = combinedText.match(regex);
      if (matches) {
        score += matches.length * weight;
      }
    }
    scores[category] = score;
  }

  let bestCategory = 'inbox';
  let bestScore = 0;
  const MINIMUM_THRESHOLD = 6;

  for (const [category, score] of Object.entries(scores)) {
    if (score > bestScore) {
      bestScore = score;
      bestCategory = category;
    }
  }

  if (bestScore < MINIMUM_THRESHOLD) {
    return 'inbox';
  }

  return bestCategory;
}

// ============== Title Generation ==============

function generateTitle(query: string, response: string): string {
  const h1Match = response.match(/^#\s+(.+)$/m);
  if (h1Match?.[1]) {
    const h1 = h1Match[1].trim();
    return h1.length > 80 ? h1.slice(0, 77) + '...' : h1;
  }

  const firstLine = response.split('\n')
    .map(l => l.trim())
    .filter(l => l.length > 10 && !l.startsWith('#') && !l.startsWith('-') && !l.startsWith('*'))
    .find(l => l.length > 0);

  if (firstLine) {
    const sentenceMatch = firstLine.match(/^(.{10,80}?)[.!?]/);
    if (sentenceMatch?.[1]) {
      return sentenceMatch[1].trim();
    }
    const words = firstLine.split(/\s+/).slice(0, 10).join(' ');
    return words.length > 80 ? words.slice(0, 77) + '...' : words;
  }

  const cleanQuery = query.trim();
  const capitalized = cleanQuery.charAt(0).toUpperCase() + cleanQuery.slice(1);
  return capitalized.length > 80 ? capitalized.slice(0, 77) + '...' : capitalized;
}

// ============== Tag Extraction ==============

function extractTags(query: string, response: string, category: string): string[] {
  const combinedText = (query + ' ' + response).toLowerCase();
  const tagCandidates: Map<string, number> = new Map();

  for (const [, keywords] of Object.entries(CATEGORY_KEYWORDS)) {
    for (const [keyword, weight] of keywords) {
      if (keyword.split(' ').length <= 2) {
        const regex = new RegExp(`\\b${keyword.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}\\b`, 'gi');
        const matches = combinedText.match(regex);
        if (matches && matches.length > 0) {
          tagCandidates.set(keyword, (tagCandidates.get(keyword) ?? 0) + matches.length * weight);
        }
      }
    }
  }

  const catEntry = CATEGORY_MAP[category];
  if (catEntry) {
    tagCandidates.set(category, 100);
  }

  const sorted = Array.from(tagCandidates.entries())
    .sort(([, a], [, b]) => b - a)
    .slice(0, 7)
    .map(([tag]) => tag.replace(/\s+/g, '-'));

  if (sorted.length < 3) {
    sorted.push('auto-documented', 'telegram-assistant');
  }

  return sorted.slice(0, 7);
}

// ============== Summary Generation ==============

function generateSummary(title: string, response: string): string {
  const lines = response.split('\n').map(l => l.trim()).filter(l => l.length > 20);
  const sentences: string[] = [];

  for (const line of lines) {
    const cleaned = line.replace(/^#{1,6}\s+/, '').replace(/^[-*]\s+/, '').trim();
    if (cleaned.length > 20) {
      const parts = cleaned.split(/(?<=[.!?])\s+/);
      for (const part of parts) {
        if (part.length > 15 && sentences.length < 6) {
          sentences.push(part);
        }
      }
    }
    if (sentences.length >= 6) break;
  }

  if (sentences.length === 0) {
    return `Document "${title}" has been automatically created from a Telegram Claude Code session.`;
  }

  return sentences.slice(0, 6).join(' ');
}

// ============== Document Structuring ==============

function structureDocument(title: string, query: string, response: string): string {
  const lines = response.split('\n');
  const resultLines: string[] = [];
  let hasHeadings = false;

  for (const line of lines) {
    if (/^#{1,6}\s+/.test(line)) {
      hasHeadings = true;
      break;
    }
  }

  if (hasHeadings) {
    let h1Replaced = false;
    for (const line of lines) {
      if (!h1Replaced && /^#\s+/.test(line)) {
        resultLines.push(`# ${title}`);
        h1Replaced = true;
      } else {
        resultLines.push(line);
      }
    }
    if (!h1Replaced) {
      resultLines.unshift(`# ${title}`, '');
    }
  } else {
    resultLines.push(`# ${title}`, '', ...lines);
  }

  while (resultLines.length > 0 && resultLines[resultLines.length - 1]?.trim() === '') {
    resultLines.pop();
  }

  const takeaways: string[] = [];
  const nonEmptyLines = lines.filter(l => l.trim().length > 10);
  for (const line of nonEmptyLines) {
    const trimmed = line.trim();
    if (/^[-*]\s+.{15,}/.test(trimmed) || /^\d+\.\s+.{15,}/.test(trimmed)) {
      const text = trimmed.replace(/^[-*\d.]\s*/, '').trim();
      if (text.length > 15) {
        takeaways.push(`- ${text}`);
      }
    }
    if (takeaways.length >= 5) break;
  }

  if (takeaways.length < 3) {
    for (const line of nonEmptyLines) {
      const trimmed = line.trim().replace(/^#{1,6}\s+/, '');
      if (trimmed.length > 20 && !trimmed.startsWith('-') && !trimmed.startsWith('*')) {
        const sentenceMatch = trimmed.match(/^(.{20,120}?)[.!?]/);
        if (sentenceMatch?.[1]) {
          takeaways.push(`- ${sentenceMatch[1].trim()}`);
        }
      }
      if (takeaways.length >= 5) break;
    }
  }

  resultLines.push('', '## Key Takeaways', '');
  if (takeaways.length > 0) {
    resultLines.push(...takeaways.slice(0, 5));
  } else {
    resultLines.push('- See full response above for details.');
  }

  resultLines.push('', '## Original Query', '', `> ${query}`);

  return resultLines.join('\n');
}

// ============== Index File Update ==============

function updateIndexFile(indexRelPath: string, filename: string, title: string): void {
  const indexPath = join(VAULT_ROOT, indexRelPath);
  if (!existsSync(indexPath)) {
    return;
  }

  const linkName = filename.replace(/\.md$/, '');
  const shortDesc = title.length > 60 ? title.slice(0, 57) + '...' : title;
  const linkLine = `[[${linkName}]] -- ${shortDesc}`;

  const indexContent = readFileSync(indexPath, 'utf-8');

  const fmMatch = indexContent.match(/^---\n[\s\S]*?\n---\n?/);
  if (fmMatch) {
    const afterFm = fmMatch[0];
    const rest = indexContent.slice(afterFm.length);
    writeFileSync(indexPath, afterFm + linkLine + '\n' + rest, 'utf-8');
  } else {
    writeFileSync(indexPath, linkLine + '\n' + indexContent, 'utf-8');
  }
}

// ============== Email Sending ==============

function sendEmail(subject: string, body: string): boolean {
  if (!EMAIL_ENABLED || !EMAIL_FROM || !EMAIL_TO) {
    return false;
  }

  try {
    const himalayaCmd = HIMALAYA_PATH;

    const rawMessage = `From: ${EMAIL_FROM}\r\nTo: ${EMAIL_TO}\r\nSubject: ${subject}\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n${body}`;

    const tmpPath = join(tmpdir(), `autodoc-email-${Date.now()}.txt`);
    writeFileSync(tmpPath, rawMessage, 'utf-8');

    try {
      execFileSync(himalayaCmd, ['message', 'send'], {
        encoding: 'utf-8',
        stdio: ['pipe', 'pipe', 'pipe'],
        input: rawMessage,
      });
    } finally {
      try { unlinkSync(tmpPath); } catch { /* best effort */ }
    }

    return true;
  } catch (err) {
    console.error('[autodoc] Email send failed:', err instanceof Error ? err.message : String(err));
    return false;
  }
}

// ============== Ensure Inbox Exists (only when enabled) ==============

if (AUTODOC_ENABLED && VAULT_ROOT) {
  mkdirSync(join(VAULT_ROOT, 'Inbox'), { recursive: true });
}

// ============== Main Export ==============

/**
 * Transform a Claude Code response into a classified Obsidian vault document.
 *
 * Returns null immediately if AUTODOC_ENABLED is not true.
 */
export async function autoDocument(query: string, response: string): Promise<AutoDocResult | null> {
  if (!AUTODOC_ENABLED || !VAULT_ROOT) {
    return null;
  }

  if (wordCount(response) < 50) {
    return null;
  }

  const trimmedResp = response.trim();
  const errorPrefixes = [
    'something went wrong',
    'error:',
    'an error occurred',
    'i apologize, but i',
    'i\'m sorry, but i',
  ];
  const lowerResp = trimmedResp.toLowerCase();
  if (errorPrefixes.some(prefix => lowerResp.startsWith(prefix))) {
    return null;
  }

  const categoryKey = classifyContent(query, response);
  const catEntry = CATEGORY_MAP[categoryKey];
  if (!catEntry) {
    console.error(`[autodoc] Unknown category key: ${categoryKey}`);
    return null;
  }

  const title = generateTitle(query, response);
  const slug = slugify(title);
  const dateStr = todayStr();
  const filename = `${dateStr}_${slug}.md`;

  const tags = extractTags(query, response, categoryKey);

  const structuredBody = structureDocument(title, query, response);

  const tagsYaml = `[${tags.map(t => t).join(', ')}]`;
  const frontmatter = [
    '---',
    `date: ${dateStr}`,
    `query: "${query.replace(/"/g, '\\"')}"`,
    `category: ${catEntry.label}`,
    `source: telegram-assistant`,
    `tags: ${tagsYaml}`,
    '---',
  ].join('\n');

  const fullDocument = frontmatter + '\n' + structuredBody + '\n';

  const docFolder = join(VAULT_ROOT, catEntry.folder);
  mkdirSync(docFolder, { recursive: true });

  const filePath = join(docFolder, filename);
  writeFileSync(filePath, fullDocument, 'utf-8');

  if (catEntry.index) {
    try {
      updateIndexFile(catEntry.index, filename, title);
    } catch (err) {
      console.error('[autodoc] Index update failed:', err instanceof Error ? err.message : String(err));
    }
  }

  const emailSent = sendEmail(title, fullDocument);

  const summary = generateSummary(title, response);

  const vaultPath = `${catEntry.folder}${filename}`;

  return {
    title,
    category: catEntry.label,
    vaultPath,
    tags,
    emailSent,
    summary,
  };
}
