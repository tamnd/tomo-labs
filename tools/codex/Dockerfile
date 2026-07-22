# The codex tool image: the shared base plus the OpenAI Codex CLI and the adapter
# that drives it. codex is installed from npm (package @openai/codex, a prebuilt
# binary), so this image is independent of any codex checkout on the host. The
# base already carries Node 22, which the npm launcher needs.
FROM tomolab-base
ARG CODEX_VERSION=0.146.0-alpha.2
RUN npm install -g @openai/codex@${CODEX_VERSION}
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
