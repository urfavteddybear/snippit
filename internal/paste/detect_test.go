package paste

import "testing"

func TestDetect(t *testing.T) {
	cases := map[string]string{
		"go":         "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tx := 1\n\tfmt.Println(x)\n}\n",
		"python":     "import os\n\ndef main():\n    print(os.getcwd())\n\nif __name__ == '__main__':\n    main()\n",
		"javascript": "const express = require('express');\nconst app = express();\napp.get('/', (req, res) => res.send('hi'));\nconsole.log('up');\n",
		"typescript": "interface User {\n  name: string;\n  age: number;\n}\nconst u: User = { name: 'a', age: 1 };\n",
		"json":       "{\"name\": \"snippit\", \"version\": 1, \"tags\": [\"a\"]}",
		"bash":       "#!/usr/bin/env bash\nset -e\necho \"hello\"\n",
		"php":        "<?php\n$name = 'x';\necho $name;\n",
		"sql":        "SELECT id, name FROM users WHERE id = 1 ORDER BY name;\n",
		"html":       "<!DOCTYPE html>\n<html><head><title>x</title></head><body><div>hi</div></body></html>\n",
		"xml":        "<?xml version=\"1.0\"?>\n<note xmlns=\"urn:x\"><to>A</to></note>\n",
		"dockerfile": "FROM golang:1.27-alpine\nWORKDIR /app\nCOPY . .\nRUN go build -o app\nCMD [\"./app\"]\n",
		"nginx":      "server {\n    listen 80;\n    server_name example.com;\n    location / {\n        proxy_pass http://app:8080;\n    }\n}\n",
		"yaml":       "version: '3'\nservices:\n  app:\n    image: snippit\n    ports:\n      - 8080:8080\n",
		"markdown":   "# Title\n\nSome **bold** text and a [link](https://example.com).\n\n```go\nfmt.Println()\n```\n",
		"rust":       "use std::io;\n\nfn main() {\n    let mut s = String::new();\n    println!(\"{}\", s);\n}\n",
		"java":       "public class Main {\n    public static void main(String[] args) {\n        System.out.println(\"hi\");\n    }\n}\n",
		"csharp":     "using System;\n\nnamespace App {\n    class P { static void Main() { Console.WriteLine(\"hi\"); } }\n}\n",
		"cpp":        "#include <iostream>\n\nint main() {\n    std::cout << \"hi\" << std::endl;\n}\n",
		"c":          "#include <stdio.h>\n\nint main(void) {\n    printf(\"hi\\n\");\n    return 0;\n}\n",
		"css":        "body {\n  margin: 0;\n  color: #fff;\n}\n@media (max-width: 600px) {\n  body { margin: 4px; }\n}\n",
		"plaintext":  "just some notes about the meeting tomorrow\n",
	}
	for want, src := range cases {
		if got := Detect(src); got != want {
			t.Errorf("Detect(%s sample) = %s", want, got)
		}
	}
}
