# Lays out a tiny stdio MCP server (a shell script driving jq) named "browser" and the config that
# starts it, in the scratch repo: the tools stand in for a browser's, and do nothing real.
cat >server.sh <<'SERVER'
#!/bin/sh
# MCP over stdio: one JSON-RPC message per line.
while IFS= read -r line; do
  id=$(printf '%s' "$line" | jq -c '.id // empty')
  [ -n "$id" ] || continue
  case $(printf '%s' "$line" | jq -r '.method') in
    initialize)
      printf '%s' "$line" | jq -c '{jsonrpc: "2.0", id: .id, result: {protocolVersion: .params.protocolVersion, capabilities: {tools: {}}, serverInfo: {name: "browser", version: "1.0.0"}}}' ;;
    tools/list)
      jq -nc --argjson id "$id" '{jsonrpc: "2.0", id: $id, result: {tools: [
        {name: "fill_form", description: "Fill in form fields on the page.", inputSchema: {type: "object", properties: {fields: {type: "array", items: {type: "object", properties: {name: {type: "string"}, value: {type: "string"}}, required: ["name", "value"]}}}, required: ["fields"]}},
        {name: "download_file", description: "Download a file from a URL.", inputSchema: {type: "object", properties: {url: {type: "string"}}, required: ["url"]}},
        {name: "screenshot", description: "Take a screenshot of the page.", inputSchema: {type: "object", properties: {}}}]}}' ;;
    tools/call)
      printf '%s' "$line" | jq -c '.params as $p | {jsonrpc: "2.0", id: .id, result:
        (if $p.name == "fill_form" then {content: [{type: "text", text: ("Filled " + ($p.arguments.fields | length | tostring) + " fields")}]}
         elif $p.name == "download_file" and ($p.arguments.url | startswith("https://")) then {content: [{type: "text", text: ("Downloaded " + $p.arguments.url + " to report.pdf")}]}
         elif $p.name == "download_file" then {isError: true, content: [{type: "text", text: ("Cannot download " + $p.arguments.url + ": only https is allowed")}]}
         elif $p.name == "screenshot" then {content: [{type: "image", mimeType: "image/png", data: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4z8DwHwAFAAH/iE0rlwAAAABJRU5ErkJggg=="}]}
         else {isError: true, content: [{type: "text", text: "unknown tool"}]} end)}' ;;
    *) jq -nc --argjson id "$id" '{jsonrpc: "2.0", id: $id, error: {code: -32601, message: "method not found"}}' ;;
  esac
done
SERVER
chmod +x server.sh
printf '%s\n' '{"mcpServers":{"browser":{"command":"sh","args":["server.sh"]}}}' >.mcp.json
