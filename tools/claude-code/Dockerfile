# The claude-code tool image: the shared base plus Anthropic's Claude Code CLI
# and the adapter that drives it. claude-code is installed from npm (package
# @anthropic-ai/claude-code, binary claude), so this image is independent of any
# checkout on the host. The base already carries Node 22, which the CLI needs.
FROM tomolab-base
ARG CLAUDE_CODE_VERSION=2.1.214
RUN npm install -g @anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
