# The aider tool image: the shared base plus the aider CLI and the adapter that
# drives it. aider is a Python program, installed from PyPI (package aider-chat,
# binary aider), so this image is independent of any aider checkout on the host.
# The base already carries Python 3 and pip; Debian marks its Python as
# externally managed, so the install passes --break-system-packages to write into
# the image's own site-packages, which is what we want in a throwaway container.
FROM tomolab-base
ARG AIDER_VERSION=0.86.2
RUN pip3 install --no-cache-dir --break-system-packages aider-chat==${AIDER_VERSION}
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
