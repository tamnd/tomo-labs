# tomolab-tb is the terminal-bench validation image: tomolab-base plus the
# handful of interpreters, compilers, and libraries a Terminal-Bench reference
# solution reaches for. The generator runs each task's solution.sh here (with no
# network) to prove the task grades, the same rich-environment the upstream
# Terminal-Bench task image would have given the agent. It is a validation image
# only; the agent still runs in its own tool image and the grade still happens on
# the host, so nothing here changes what a solving agent is measured against.
FROM tomolab-base

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
      python-is-python3 python3-dev \
      gcc g++ \
      bzip2 xz-utils zip unzip \
      sqlite3 \
      bsdmainutils vim-common \
      tree \
    && rm -rf /var/lib/apt/lists/*

# The third-party libraries a grader or a reference solution commonly imports.
# Kept to the same allowlist the generator installs into the host grading venv, so
# a solution that runs here and a grader that runs on the host see the same names.
RUN pip3 install --no-cache-dir --break-system-packages \
      numpy pandas scipy sympy networkx rdflib python-chess pyyaml \
      python-dateutil beautifulsoup4 jsonschema pytz requests \
      python-sat
