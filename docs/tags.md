# Tags

Tags help you search across cheats. They're not a separate query syntax; they
just fold into the same picker search as titles, descriptions, and commands.
Type any tag word and matching cheats are filtered.

## Where tags come from

A single cheat can pick up tags from five sources, merged together.

### 1. Folder and file path

Every directory and filename above a cheat becomes a tag automatically. A
cheat at `cloud/aws/s3.md` gets `cloud`, `aws`, `s3` for free.

This is the cheapest tag system. Organize your cheats into folders and you're
already done.

### 2. YAML front matter (file-wide)

A YAML block at the very top of the file. Applies to every cheat in the file.

Inline list:

```markdown
---
tags: [aws, cloud]
---
```

Block list:

```markdown
---
tags:
  - aws
  - cloud
---
```

### 3. Footer block (file-wide)

A tag block at the end of the file. Same scope as front matter. Two shapes
are accepted:

Hashtag block (most common):

```markdown
## last cheat

\`\`\`sh title:"..."
...
\`\`\`

---
#quickref #production #internal
```

The `---` rule is optional. Multiple hashtag lines also work.

YAML form (same as front matter):

```markdown
---
tags: [quickref, production]
---
```

### 4. Inline `#tag` in prose (per cheat)

Hashtags placed in the prose between a `##` heading and the next `##` heading
attach to *that cheat only*. Works above or below the code block.

```markdown
## list buckets

List all S3 buckets in the account. #s3

\`\`\`sh title:"List S3 buckets"
aws s3 ls
\`\`\`

#read-only

## describe instance
...
```

Both `#s3`, and `#read-only` attach to "list buckets". They do not
leak into "describe instance".

Rules for what counts as an inline tag:

- Must start with `#` followed by an ASCII letter. Rules out hex colors
  (`#ff0000`), issue refs (`#42`), and bare `#`.
- Tag body can contain letters, digits, `_`, `-`, `.`, `/`.
- Must be preceded by whitespace, start-of-line, or `( [ ,`.
- Markdown heading lines (`# foo`, `## foo`) are not scanned.
- Code fences are not scanned.

### 5. The heading itself

Words in a `## heading` are already searchable, so you can put short hints
right in the title:

```markdown
## list buckets #s3
```

## How they merge

For each cheat, the final tag set is the union of:

```
path + front matter + footer + inline-under-heading
```

Then lowercased and deduplicated. Order within a cheat is path tags first,
then file tags, then inline tags.
