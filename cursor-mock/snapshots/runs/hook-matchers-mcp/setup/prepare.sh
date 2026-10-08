mkdir -p .cursor
cat > .cursor/echo-mcp.sh <<'EOF'
#!/bin/sh
# A minimal MCP server over stdio: one tool, echo.
while IFS= read -r line; do
  method=$(printf '%s' "$line" | jq -r '.method // ""')
  id=$(printf '%s' "$line" | jq -c '.id')
  case "$method" in
    initialize)
      jq -nc --argjson id "$id" '{jsonrpc:"2.0",id:$id,result:{protocolVersion:"2024-11-05",capabilities:{tools:{}},serverInfo:{name:"local",version:"1"}}}' ;;
    tools/list)
      jq -nc --argjson id "$id" '{jsonrpc:"2.0",id:$id,result:{tools:[{name:"echo",description:"Echoes its text argument back.",inputSchema:{type:"object",properties:{text:{type:"string"}},required:["text"]}}]}}' ;;
    tools/call)
      text=$(printf '%s' "$line" | jq -r '.params.arguments.text // ""')
      jq -nc --argjson id "$id" --arg t "ECHOED: $text" '{jsonrpc:"2.0",id:$id,result:{content:[{type:"text",text:$t}]}}' ;;
    ping)
      jq -nc --argjson id "$id" '{jsonrpc:"2.0",id:$id,result:{}}' ;;
  esac
done
EOF
chmod +x .cursor/echo-mcp.sh
printf '{"mcpServers":{"local":{"command":"sh","args":[".cursor/echo-mcp.sh"]}}}\n' > .cursor/mcp.json
