FROM golang:1.16-alpine as golangAlpine

RUN apk add curl && apk add --virtual build-dependencies build-base gcc

ARG INSTALL_DIR=/opt/harvest
ARG BUILD_DIR=/opt/home
ARG VERSION=2.0
ARG RELEASE=nightly

RUN echo $VERSION
RUN echo $RELEASE

# Set the Current Working Directory inside the container
WORKDIR $BUILD_DIR

RUN mkdir -p $INSTALL_DIR

COPY . .

RUN make build VERSION=$VERSION RELEASE=$RELEASE

RUN cp -a $BUILD_DIR/harvest.yml $INSTALL_DIR/harvest.yml

RUN cp -aR bin $BUILD_DIR/conf $BUILD_DIR/docs $BUILD_DIR/grafana $BUILD_DIR/docker/allPollerInOneContainer/docker-entrypoint.sh $INSTALL_DIR && \
   chmod +x $INSTALL_DIR/docker-entrypoint.sh

FROM alpine:latest

RUN apk add bash

ARG INSTALL_DIR=/opt/harvest
COPY --from=golangAlpine $INSTALL_DIR $INSTALL_DIR
WORKDIR $INSTALL_DIR

VOLUME     [ "/opt/harvest" ]
VOLUME     [ "/var/log/harvest" ]

ENTRYPOINT ["./docker-entrypoint.sh"]
CMD ["start", "--config", "./harvest.yml"]
