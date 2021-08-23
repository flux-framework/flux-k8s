#                                               -*- Autoconf -*-
# Process this file with autoconf to produce a configure script.
AC_PREREQ([2.63])
AC_INIT([flux-kube], 
         m4_esyscmd([git describe --always | awk '/.*/ {sub(/^v/, ""); printf "%s",$1; exit}']))

AC_CONFIG_AUX_DIR([config])
AC_CONFIG_MACRO_DIR([config])
AC_CONFIG_SRCDIR([NEWS])

AM_INIT_AUTOMAKE([subdir-objects tar-ustar filename-length-max=256 foreign])
AM_SILENT_RULES([yes])
AM_MAINTAINER_MODE([enable])

AC_PREFIX_PROGRAM([flux])

LT_INIT([dlopen])

PKG_PROG_PKG_CONFIG

# Checks for programs.
AC_DEFINE([_GNU_SOURCE], 1,
          [Define _GNU_SOURCE so that we get all necessary prototypes])
AC_PROG_CC
AC_PROG_CC_C99
AC_PROG_LN_S
AC_PROG_MAKE_SET
AM_PROG_CC_C_O

# We can add more checks in this section
AX_FLUX_CORE

#  Set PYTHON_VERSION to FLUX_PYTHON_VERSION here
PYTHON_VERSION=${PYTHON_VERSION:-$FLUX_PYTHON_VERSION}
AX_PYTHON_DEVEL([>='3.6'])

AM_PATH_PYTHON([$ac_python_version])
if test "X$PYTHON" = "X"; then
  AC_MSG_ERROR([could not find python])
fi

AM_CHECK_PYMOD(openshift,
               [StrictVersion(openshift.__version__) >= StrictVersion('0.11.2')], [],
               [AC_MSG_ERROR([[could not find python module openshift, version 0.11.2+ required]])])

#
# Project directories
#
AS_VAR_SET(fluxcmddir, $libexecdir/flux/cmd)
AC_SUBST(fluxcmddir)

AS_VAR_SET(jobtap_plugindir, $libdir/flux/job-manager/plugins)
AC_SUBST(jobtap_plugindir)

AS_VAR_SET(fluxrc1dir, $sysconfdir/flux/rc1.d)
AC_SUBST(fluxrc1dir)

##
# Macros to avoid repetition in Makefiles.am's
##
fluxplugin_ldflags="-avoid-version -export-symbols-regex '^flux_plugin_init\$\$' --disable-static -shared -export-dynamic -module"
AC_SUBST(fluxplugin_ldflags)

# Tells automake to create a Makefile
# See https://www.gnu.org/software/automake/manual/html_node/Requirements.html
AC_CONFIG_FILES([Makefile
  cmd/Makefile
  lib/Makefile
  plugins/Makefile
  etc/Makefile])

# Generate the output
AC_OUTPUT
