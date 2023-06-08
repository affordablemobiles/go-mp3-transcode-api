FROM golang:1.20-bullseye as build-env

RUN apt-get update -qq && apt-get -y install \
  autoconf \
  automake \
  build-essential \
  cmake \
  git-core \
  libass-dev \
  libfreetype6-dev \
  libgnutls28-dev \
  libmp3lame-dev \
  libtool \
  libvorbis-dev \
  meson \
  ninja-build \
  pkg-config \
  texinfo \
  wget \
  yasm \
  zlib1g-dev \
  libunistring-dev \
  libaom-dev \
  libdav1d-dev \
  nasm \
  wget

RUN cd /usr/src && \
    wget -Offmpeg.tar.gz https://github.com/FFmpeg/FFmpeg/archive/refs/tags/n4.4.4.tar.gz && \
    tar -xf ffmpeg.tar.gz && \
    cd FFmpeg-n4.4.4 && \
    ./configure --prefix=/usr/local/ffmpeg --enable-shared --enable-libmp3lame --enable-nonfree && \
    make -j$(nproc) && \
    make install

ADD . /go/src/bitbucket.org/a1commsltd/mp3-transcode
WORKDIR /go/src/bitbucket.org/a1commsltd/mp3-transcode

RUN go mod vendor
RUN PKG_CONFIG_PATH=$PKG_CONFIG_PATH:/usr/local/ffmpeg/lib/pkgconfig/ go build -ldflags "-s -w" -o /go/bin/app

FROM debian:bullseye
RUN apt-get update -qq && apt-get -y install \
  libass9 \
  libfreetype6 \
  libgnutlsxx28 libgnutls30 libgnutls-dane0 libgnutls-openssl27 \
  libmp3lame0 \
  libvorbis0a \
  zlib1g \
  libunistring2 \
  libaom0 \
  libdav1d4
COPY --from=build-env /usr/local/ffmpeg/ /usr/local/ffmpeg/
RUN echo "/usr/local/ffmpeg/lib" > /etc/ld.so.conf.d/ffmpeg.conf && ldconfig
RUN apt-get -y install ca-certificates
COPY --from=build-env /go/bin/app /
COPY demo.html /demo.html
CMD ["/app"]
