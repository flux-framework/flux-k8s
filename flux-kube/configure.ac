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

# Check for C compiler
# AC_PROG_CC
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

# Tells automake to create a Makefile
# See https://www.gnu.org/software/automake/manual/html_node/Requirements.html
AC_CONFIG_FILES([Makefile
  cmd/Makefile
  lib/Makefile])

# Generate the output
AC_OUTPUT
