AM_CFLAGS = \
	$(WARNING_CFLAGS) 

AM_CPPFLAGS = \
	-I$(FLUX_PREFIX) \
	-I$(FLUX_PREFIX)/src/include \
	-I$(FLUX_PREFIX)/src/common/libflux \
	$(ZMQ_CFLAGS) $(JANSSON_CFLAGS)

jobtap_plugin_LTLIBRARIES = \
	flux-kube.la

flux_kube_la_SOURCES = \
	flux-kube.c

flux_kube_la_LDFLAGS = \
	$(fluxplugin_ldflags) \
	-module
