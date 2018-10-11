#!/bin/bash
bldtmp='/tmp'
approot='/opt/tg2sip'

apt-get install -y --no-install-recommends \
		build-essential git \
		wget ca-certificates \
		pkg-config libopus-dev libssl-dev \
		zlib1g-dev gperf ccache cmake \
&& cp tdlib_header.patch ${bldtmp}/ \
&& cp tdlib_threadname.patch ${bldtmp}/ \
&& git clone https://github.com/tdlib/td.git \
	&& cd td \
	&& git reset --hard v1.2.0 \
	&& git apply ${bldtmp}/tdlib_header.patch \
	&& git apply ${bldtmp}/tdlib_threadname.patch \
	&& mkdir build \
	&& cd build \
	&& cmake -DCMAKE_BUILD_TYPE=Release .. \
	&& cmake --build . --target install \
	&& cd ../.. \
	&& rm -rf td \
&& cp config_site.h ${bldtmp}/ \
&& git clone https://github.com/Infactum/pjproject.git \
	&& cd pjproject \
	&& cp ${bldtmp}/config_site.h pjlib/include/pj \
	&& ./configure --disable-sound CFLAGS="-O3 -DNDEBUG" \
	&& make dep && make && make install \
	&& cd .. \
	&& rm -rf pjproject \
&& git clone -n https://github.com/gabime/spdlog.git \
	&& cd spdlog \
	&& git checkout tags/v0.17.0 \
	&& mkdir build \
	&& cd build \
	&& cmake -DCMAKE_BUILD_TYPE=Release -DSPDLOG_BUILD_EXAMPLES=OFF -DSPDLOG_BUILD_TESTING=OFF .. \
	&& cmake --build . --target install \
	&& cd ../.. \
	&& rm -rf spdlog \
&& git clone https://github.com/D4rk4/tg2sip.git \
	&& cd tg2sip \
	&& mkdir build    \
    	&& cd build    \
	&& cmake -DCMAKE_BUILD_TYPE=Release ..  \
	&& cmake --build . \
	&& cd ../.. \
	&& mkdir -p ${approot} \
	&& cp tg2sip/build/tg2sip ${approot}/ \
	&& cp tg2sip/build/gen_db ${approot}/ \
	&& cp tg2sip/build/settings.ini ${approot}/ \
	&& rm -rf tg2sip
