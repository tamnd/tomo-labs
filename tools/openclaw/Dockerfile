# The openclaw tool image: the shared base plus the openclaw CLI and the
# adapter that drives it. openclaw is installed from npm, so this image is
# independent of any openclaw checkout on the host. The base already carries
# Node 22, which openclaw needs.
FROM tomolab-base
ARG OPENCLAW_VERSION=2026.7.1-2
RUN npm install -g openclaw@${OPENCLAW_VERSION}
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
