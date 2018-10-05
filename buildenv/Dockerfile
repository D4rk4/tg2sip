FROM debian:sid as builder

RUN apt-get update \
	&& apt-get install -y --no-install-recommends \
		build-essential git \
		wget ca-certificates \
		pkg-config libopus-dev libssl-dev \
		zlib1g-dev gperf ccache cmake \
	&& rm -rf /var/lib/apt/lists/*

#RUN wget https://cmake.org/files/v3.9/cmake-3.9.6-Linux-x86_64.sh \
#    && sh cmake-3.9.6-Linux-x86_64.sh --prefix=/usr --exclude-subdir

COPY tdlib_header.patch /
COPY tdlib_threadname.patch /

RUN git clone https://github.com/tdlib/td.git \
    && cd td \
    && git reset --hard v1.2.0 \
    && git apply /tdlib_header.patch \
    && git apply /tdlib_threadname.patch \
    && mkdir build \
    && cd build \
    && cmake -DCMAKE_BUILD_TYPE=Release .. \
    && cmake --build . --target install \
    && cd / \
    && rm -rf td

COPY config_site.h /

RUN git clone https://github.com/Infactum/pjproject.git \
    && cd pjproject \
    && cp /config_site.h pjlib/include/pj \
    && ./configure --disable-sound CFLAGS="-O3 -DNDEBUG" \
    && make dep && make && make install \
    && cd / \
    && rm -rf pjproject

RUN git clone -n https://github.com/gabime/spdlog.git \
    && cd spdlog \
    && git checkout tags/v0.17.0 \
    && mkdir build \
    && cd build \
    && cmake -DCMAKE_BUILD_TYPE=Release -DSPDLOG_BUILD_EXAMPLES=OFF -DSPDLOG_BUILD_TESTING=OFF .. \
    && cmake --build . --target install \
    && cd / \
    && rm -rf spdlog


RUN git clone https://github.com/D4rk4/tg2sip.git \
    && cd tg2sip \
    && mkdir build    \
    && cd build    \
    && cmake -DCMAKE_BUILD_TYPE=Release ..  \
    && cmake --build .

RUN ldd /tg2sip/build/tg2sip

FROM debian:stretch

RUN apt-get update \
	&& apt-get install -y --no-install-recommends \
		libopus0 libssl1.1\
	&& rm -rf /var/lib/apt/lists/*

WORKDIR /gateway

COPY --from=builder /tg2sip/build/tg2sip .
COPY --from=builder /tg2sip/build/gen_db .
COPY --from=builder /tg2sip/build/settings.ini .

ENTRYPOINT ./tg2sip
