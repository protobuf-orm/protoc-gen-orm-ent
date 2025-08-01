FROM ghcr.io/lesomnus/dev-golang:1.24

RUN TEMP=$(mktemp -d); \
	ARCH=$(uname -m); \
	VERSION="31.1"; \
	case "$ARCH" in \
		'x86_64') ;; \
		'aarch64') \
			ARCH='aarch_64'; \
			;; \
		*) exit 1 ;; \
	esac; \
	cd "${TEMP}" \
	&& curl -o ./protoc.zip -fsSL "https://github.com/protocolbuffers/protobuf/releases/download/v${VERSION}/protoc-${VERSION}-linux-${ARCH}.zip" \
	&& unzip ./protoc.zip -d /usr/local \
	&& chmod 755 /usr/local/bin/protoc \
	&& rm -rf "${TEMP}"

RUN VERSION="1.55.1"; \
	curl -fsSL "https://github.com/bufbuild/buf/releases/download/v${VERSION}/buf-$(uname -s)-$(uname -m).tar.gz" \
		| tar -xvzf - -C /usr/local --strip-components 1 buf/etc buf/bin buf/share/zsh

RUN --mount=type=cache,target=/home/hypnos/.cache/go-build \
	--mount=type=cache,target=/go/pkg/mod \
	--mount=type=cache,target=/go/pkg/sumdb \
	echo \
		google.golang.org/protobuf/cmd/protoc-gen-go@v1.36 \
		google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5 \
	| xargs -n1 go install



USER hypnos

RUN mkdir -p \
		/go/pkg/mod \
		/home/hypnos/.cache/go-build

# Comment out the following lines if you don't want to use Zsh.
RUN ZSH_CUSTOM="${HOME}/.oh-my-zsh/custom"; \
	wget https://github.com/robbyrussell/oh-my-zsh/raw/master/tools/install.sh -O - | zsh || true \
	&& git clone https://github.com/zsh-users/zsh-autosuggestions         "${ZSH_CUSTOM}/plugins/zsh-autosuggestions"     \
	&& git clone https://github.com/zsh-users/zsh-syntax-highlighting.git "${ZSH_CUSTOM}/plugins/zsh-syntax-highlighting" \
	&& git clone https://github.com/zsh-users/zsh-completions             "${ZSH_CUSTOM}/plugins/zsh-completions"
COPY .zshrc /home/hypnos
ENV SHELL=/bin/zsh
