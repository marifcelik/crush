Launch a new agent that has access to the following tools: glob, grep, ls, view. When you are searching for a keyword or file and are not confident that you will find the right match on the first try, use the agent tool to perform the search for you.
{{ if .CanReason -}}
You can set `reasoning_effort` to tune the sub-agent's thinking: "off" for trivial tasks such as summarization, one of {{ .ReasoningLevels }}, or "max" for the highest level when the task requires hard reasoning.
{{- end }}
{{ if .HasVision -}}
The sub-agent's model supports vision: its view tool can inspect image files (PNG, JPEG, GIF, WebP), so you can delegate image analysis or description tasks to it.
{{- end }}
