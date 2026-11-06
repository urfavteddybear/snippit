package paste

import (
	"encoding/json"
	"regexp"
	"strings"
)

// detectSample caps how much content the detector inspects.
const detectSample = 16 * 1024

type rule struct {
	re     *regexp.Regexp
	weight int
}

func r(pattern string, weight int) rule {
	return rule{regexp.MustCompile(pattern), weight}
}

// detectRules are cheap heuristics; the highest score wins.
// Line-anchored patterns use (?m).
var detectRules = map[string][]rule{
	"go": {
		r(`(?m)^package \w+\s*$`, 6), r(`(?m)^func (\(\w+ \*?\w+\) )?\w+\(`, 4),
		r(`:= `, 2), r(`(?m)^import \(`, 3), r(`\bfmt\.\w+\(`, 3), r(`\bchan\b|\bgo func\b`, 2),
	},
	"rust": {
		r(`\bfn \w+\s*(<[^>]*>)?\(`, 3), r(`\blet mut\b`, 4), r(`\bimpl\b.*\{`, 3),
		r(`(?m)^use \w+(::\w+)+`, 4), r(`\w+!\(`, 2), r(`&mut self|&self`, 4), r(`->\s*\w+.*\{`, 1),
	},
	"python": {
		r(`(?m)^\s*def \w+\(.*\)\s*(->.*)?:\s*$`, 5), r(`(?m)^\s*(from \w+(\.\w+)* )?import \w+`, 2),
		r(`(?m)^\s*class \w+(\(.*\))?:\s*$`, 5), r(`\bself\.`, 2), r(`(?m)^\s*(elif|except|with) .*:\s*$`, 4),
		r(`\bprint\(`, 1), r(`if __name__ == ['"]__main__['"]`, 8), r(`\bNone\b|\bTrue\b|\bFalse\b`, 1),
	},
	"javascript": {
		r(`\b(const|let|var) \w+ = `, 2), r(`\bfunction\s*\w*\(`, 2), r(`=>`, 1),
		r(`\bconsole\.\w+\(`, 3), r(`\brequire\(['"]`, 3), r(`\bmodule\.exports\b`, 4),
		r(`\bdocument\.|\bwindow\.`, 3), r(`(?m)^import .* from ['"]`, 2), r(`===|!==`, 2),
	},
	"typescript": {
		r(`(?m)^\s*(export )?interface \w+`, 5), r(`(?m)^\s*(export )?type \w+ = `, 4),
		r(`:\s*(string|number|boolean|void|any|unknown)\b`, 3), r(`\bas const\b`, 3),
		r(`(?m)^import .* from ['"]`, 1), r(`\b(public|private|readonly) \w+:`, 3), r(`<\w+>\(`, 1),
	},
	"java": {
		r(`\bpublic (static )?(final )?class \w+`, 4), r(`\bpublic static void main\(String`, 8),
		r(`\bSystem\.out\.print`, 6), r(`(?m)^import java\.`, 6), r(`(?m)^package [\w.]+;`, 5), r(`@Override`, 4),
	},
	"csharp": {
		r(`(?m)^using System(\.\w+)*;`, 7), r(`\bnamespace [\w.]+`, 3), r(`\bConsole\.Write`, 6),
		r(`\bpublic (static )?(async )?\w+ \w+\(.*\)\s*$`, 1), r(`\{ get; (private )?set; \}`, 6), r(`\bvar \w+ = new\b`, 2),
	},
	"c": {
		r(`(?m)^#include <\w+\.h>`, 5), r(`\bprintf\(`, 3), r(`\bint main\(`, 3),
		r(`\bmalloc\(|\bfree\(`, 3), r(`\bstruct \w+ \{`, 2), r(`(?m)^#define `, 2),
	},
	"cpp": {
		r(`(?m)^#include <\w+>`, 5), r(`\bstd::`, 5), r(`\bcout\s*<<`, 5),
		r(`\btemplate\s*<`, 4), r(`\bnamespace \w+\s*\{`, 2), r(`\bclass \w+\s*(:\s*public \w+)?\s*\{`, 2),
	},
	"php": {
		r(`<\?php`, 12), r(`\$\w+\s*=`, 2), r(`\becho\b`, 1), r(`->\w+\(`, 1), r(`\bfunction \w+\(\$`, 4),
	},
	"bash": {
		r(`(?m)^#!.*\b(ba|z)?sh\b`, 12), r(`(?m)^\s*(if|while) \[{1,2} `, 4), r(`(?m)^\s*(fi|done|esac)\s*$`, 4),
		r(`\$\{?\w+\}?`, 1), r(`(?m)^\s*(export|echo|sudo|apt|cd|mkdir|chmod) `, 2), r(`\|\s*(grep|awk|sed|xargs)\b`, 3),
	},
	"sql": {
		r(`(?i)\bSELECT\b[\s\S]+?\bFROM\b`, 5), r(`(?i)\bCREATE (TABLE|INDEX|VIEW|DATABASE)\b`, 6),
		r(`(?i)\bINSERT INTO\b`, 6), r(`(?i)\bUPDATE \w+ SET\b`, 6), r(`(?i)\b(WHERE|JOIN|GROUP BY|ORDER BY)\b`, 2),
		r(`(?i)\bALTER TABLE\b|\bDROP TABLE\b`, 6),
	},
	"html": {
		r(`(?i)<!DOCTYPE html`, 12), r(`(?i)<html[\s>]`, 8), r(`(?i)<(div|span|body|head|script|a|p|ul|li)[\s>]`, 3),
		r(`(?i)</(div|span|body|head|p|ul|li)>`, 3),
	},
	"xml": {
		r(`(?m)^<\?xml `, 12), r(`<\w+:\w+[\s>]`, 3), r(`xmlns(:\w+)?=`, 5), r(`<!\[CDATA\[`, 5),
	},
	"css": {
		r(`(?m)^[.#]?[\w-]+(\s*[.#:>,]\s*[\w-]+)*\s*\{\s*$`, 3), r(`(?m)^\s*[\w-]+:\s*[^;{]+;\s*$`, 2),
		r(`@media\b|@import\b|@keyframes\b`, 5), r(`!important`, 3), r(`#[0-9a-fA-F]{3,6};`, 2),
	},
	"yaml": {
		r(`(?m)^---\s*$`, 3), r(`(?m)^[\w.-]+:\s*$`, 2), r(`(?m)^[\w.-]+: \S`, 2),
		r(`(?m)^\s+- [\w.-]+`, 1), r(`(?m)^(apiVersion|kind|services|version|on|jobs):`, 5),
	},
	"markdown": {
		r(`(?m)^#{1,6} \S`, 4), r("(?m)^```", 4), r(`\[[^\]]+\]\([^)]+\)`, 3),
		r(`(?m)^\s*[-*] \[[ x]\] `, 4), r(`\*\*[^*]+\*\*`, 2), r(`(?m)^> `, 2),
	},
	"dockerfile": {
		r(`(?m)^FROM [\w./:@-]+`, 8), r(`(?m)^(RUN|COPY|ADD|WORKDIR|EXPOSE|ENTRYPOINT|CMD|ENV|ARG) `, 4),
	},
	"nginx": {
		r(`(?m)^\s*server\s*\{`, 6), r(`(?m)^\s*location\s+[^{]+\{`, 6), r(`(?m)^\s*(listen|server_name|proxy_pass|root)\s+[^;]+;`, 5),
		r(`(?m)^\s*(http|upstream \w+|events)\s*\{`, 5),
	},
}

// Detect guesses the language of content. It never fails; unknown text is plaintext.
func Detect(content string) string {
	s := content
	if len(s) > detectSample {
		s = s[:detectSample]
	}
	t := strings.TrimSpace(s)
	if t == "" {
		return "plaintext"
	}

	if strings.HasPrefix(t, "#!") {
		first, _, _ := strings.Cut(t, "\n")
		switch {
		case strings.Contains(first, "python"):
			return "python"
		case strings.Contains(first, "node"):
			return "javascript"
		case strings.Contains(first, "sh"):
			return "bash"
		}
	}
	if (t[0] == '{' || t[0] == '[') && len(content) <= detectSample && json.Valid([]byte(t)) {
		return "json"
	}

	best, bestScore := "plaintext", 0
	for lang, rules := range detectRules {
		score := 0
		for _, rl := range rules {
			if n := len(rl.re.FindAllStringIndex(s, 5)); n > 0 {
				score += rl.weight * min(n, 3)
			}
		}
		// Ties resolve alphabetically so results are deterministic.
		if score > bestScore || score == bestScore && score > 0 && lang < best {
			best, bestScore = lang, score
		}
	}
	// TypeScript is a superset of JavaScript: prefer it only with type evidence.
	if best == "javascript" {
		ts := 0
		for _, rl := range detectRules["typescript"][:4] {
			if rl.re.MatchString(s) {
				ts++
			}
		}
		if ts > 0 {
			best = "typescript"
		}
	}
	if bestScore < 5 {
		return "plaintext"
	}
	return best
}
