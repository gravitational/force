.PHONY: build
build:
	yarn run build

.PHONY: start
start:
	yarn run start

# installs necessary development and build tools for
# typescript frontend development
# https://www.typescriptlang.org/docs/handbook/react-&-webpack.html
.PHONY: install
install:
# install webpack 
	yarn add --dev webpack webpack-cli webpack-dev-server html-webpack-plugin copy-webpack-plugin string-replace-loader
# react
	yarn add react redux react-redux
# react typescript types
	yarn add --dev @types/react @types/react-dom @types/react-redux
# typescript
	yarn add --dev typescript ts-loader source-map-loader
# grpc
	yarn add google-protobuf @improbable-eng/grpc-web
# grpc dev types
	yarn add --dev @types/google-protobuf ts-protoc-gen


