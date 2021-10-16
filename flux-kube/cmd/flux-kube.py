##############################################################
# Copyright 2021 Lawrence Livermore National Security, LLC
# (c.f. AUTHORS, NOTICE.LLNS, COPYING)
#
# This file is part of the Flux resource manager framework.
# For details, see https://github.com/flux-framework.
#
# SPDX-License-Identifier: LGPL-3.0
##############################################################

import logging
import argparse
import urllib3
import yaml
import subprocess
import errno

from kube_translator import JobspecTranslator
from kubernetes import client, config
from openshift.dynamic import DynamicClient

import flux
from flux import util
from flux.constants import FLUX_MSGTYPE_REQUEST


class KubeCmd:
    """
    KubeCmd is the base class for all flux-kube subcommands. 
    While all derived classes interface with K8s and OpenShift, 
    much of the functionality requires OpenShift templates. 
    The functionality will be extended to interface with 
    base K8s CRDs (which can replicate the template functionality) 
    in the future.
    """

    def __init__(self, **kwargs):
        self.parser = self.create_parser()

    @staticmethod
    def create_parser():
        """
        Create default parser for kube subcommands
        """
        parser = argparse.ArgumentParser(add_help=False)

        return parser

    def get_parser(self):
        return self.parser


class InitKube(KubeCmd):
    """
    InitKube initializes Kube/Openshift
    """

    def __init__(self, **kwargs):
        super().__init__()

        self.parser.add_argument(
            "-n",
            "--namespace",
            type=str,
            default="openshift",
            metavar="N",
            help="OpenShift/K8s namespace",
        )
        k8s_client = config.new_client_from_config()
        try:
            dyn_client = DynamicClient(k8s_client)
        except client.rest.ApiException as rest_exception:
            if rest_exception.status == 401:
                raise Exception(
                    "You must be logged in to the K8s or OpenShift"
                    " cluster to continue"
                )
            raise
        self.dyn_client = dyn_client

    def main(self, args):
        try:
            template = self.dyn_client.resources.get(
                api_version="template.openshift.io/v1", kind="Template"
            )
            templates = template.get(namespace=args.namespace)
        except client.rest.ApiException as rest_exception:
            if rest_exception.status == 401:
                raise Exception(
                    "You do not have permission to get templates",
                )
            raise
        else:
            self.templates = templates


class GetCmd(InitKube):
    """
    GetCmd gets templates and their
    values.
    """

    def __init__(self):
        super().__init__()

        self.parser.add_argument(
            "get_object",
            metavar="G",
            type=str,
            help="print OpenShift object or attribute, supported arguments:\n"
            "template_names: names of available OpenShift templates\n"
            "template_params: OpenShift template parameters\n"
            "templates: OpenShift templates\n"
            "any other OpenShift object, e.g., pods or services\n",
        )

    def main(self, args):
        super().main(args)

        if args.get_object == "template_names":
            for n in range(len(self.templates["items"])):
                print(self.templates["items"][n]["metadata"]["name"])
        elif args.get_object == "template_params":
            for n in range(len(self.templates["items"])):
                print(
                    self.templates["items"][n]["metadata"]["name"] + "\n",
                    self.templates["items"][n]["parameters"],
                )
                print("")
        elif args.get_object == "templates":
            for t in self.templates["items"]:
                print(t)
                print("")
        else:
            try:
                output = subprocess.check_output(("oc", "get", args.get_object))
            except:
                raise
            else:
                print(output.decode("utf-8").replace("\\n", "\n"))


class TranslateCmd(InitKube):
    """
    TranslateCmd translates a Flux jobspec V1
    into the corresponding K8s/OpenShift template
    and applies template overrides.
    """

    def __init__(self):
        super().__init__()

        self.logger = logging.getLogger("flux-kube")
        self.parser.add_argument(
            "-j",
            "--jobspec",
            type=str,
            metavar="J",
            help="path to Flux jobspec",
        )
        self.parser.add_argument(
            "-s",
            "--string",
            type=str,
            metavar="S",
            help="Flux jobspec string",
        )

    def main(self, args):
        super().main(args)

        if args.jobspec:
            try:
                with open(args.jobspec, "r") as f:
                    self.jobspec = yaml.safe_load(f)
            except IOError as e:
                self.logger.error(e)
                raise
        elif args.string:
            try:
                tmp_jobspec = yaml.safe_load(args.string)
                if "jobspec" in tmp_jobspec:
                    self.jobspec = tmp_jobspec["jobspec"]
                else:
                    self.jobspec = tmp_jobspec
            except Exception as e:
                self.logger.error(e)
                raise

        # Transform templates to be keyed by service name to pass
        # to translation library
        self.trans_templates = {}
        for n in range(len(self.templates["items"])):
            self.trans_templates[
                (self.templates["items"][n]["metadata"]["name"])
            ] = self.templates["items"][n]
        try:
            js_translator = JobspecTranslator(self.jobspec, self.trans_templates)
            self.service = js_translator.translate()
        except Exception as e:
            self.logger.error(e)
            raise


class SubmitCmd(TranslateCmd):
    """
    SubmitCmd translates a Flux jobspec V1
    into the corresponding K8s/OpenShift template
    and applies template overrides. Then it
    creates the corresponding objects via `oc create`.
    """

    def __init__(self):
        super().__init__()

    def main(self, args):
        super().main(args)

        cmd = [
            "oc",
            "process",
            args.namespace + "//" + self.service["template"]["metadata"]["name"],
        ]

        for k, v in self.service["overrides"].items():
            cmd.extend(("-p", k + "=" + v))

        try:
            ps = subprocess.Popen(cmd, stdout=subprocess.PIPE)
            output = subprocess.check_output(
                ("oc", "create", "-f", "-"), stdin=ps.stdout, stderr=subprocess.STDOUT
            )
            ps.wait()
        except subprocess.CalledProcessError as e:
            if "AlreadyExists" in str(e.stdout):
                pass
            else:
                self.logger.error(e)
                raise


class CancelCmd(TranslateCmd):
    """
    CancelCmd translates a Flux jobspec V1
    into the corresponding K8s/OpenShift template
    and applies template overrides. Then it
    deletes the corresponding objects via `oc delete`.
    """

    def __init__(self):
        super().__init__()

    def main(self, args):
        super().main(args)

        cmd = [
            "oc",
            "process",
            args.namespace + "//" + self.service["template"]["metadata"]["name"],
        ]

        for k, v in self.service["overrides"].items():
            cmd.extend(("-p", k + "=" + v))

        try:
            ps = subprocess.Popen(cmd, stdout=subprocess.PIPE)
            (
                subprocess.check_output(
                    ("oc", "delete", "-f", "-"),
                    stdin=ps.stdout,
                    stderr=subprocess.STDOUT,
                )
            )
            ps.wait()
        except subprocess.CalledProcessError as e:
            if "Error from server (NotFound)" in str(e.output):
                self.logger.warning(
                    "Can't cancel nonexistent objects:\n" + str(e.output)
                )
            else:
                self.logger.error(e)
                raise
        except Exception as e:
            self.logger.error(e)
            raise


class MiniCmd(KubeCmd):
    """
    MiniCmd takes a template name and
    overrides as arguments and constucts a skeleton
    jobspec based on the arguments. It then invokes
    SubmitCmd to create the objects or CancelCmd
    to delete them.
    """

    def __init__(self):
        super().__init__()

        self.logger = logging.getLogger("flux-kube")
        self.parser.add_argument(
            "mini",
            type=str,
            metavar="M",
            help="mini command: submit or cancel",
        )
        self.parser.add_argument(
            "template_name",
            type=str,
            metavar="T",
            help="template to submit/cancel",
        )
        self.parser.add_argument(
            "-o",
            "--overrides",
            type=str,
            metavar="OR",
            default="{}",
            help="template overrides",
        )

    def main(self, args):
        # Dummy jobspec that gets translated to a template with overrides
        jobspec = (
            '{"tasks": [{"command": ["' + args.template_name + '"]}], '
            '"attributes": {"system": {"flux-kube": 1, '
            '"template_overrides": ' + args.overrides + "}}}"
        )

        if args.mini == "submit":
            submit = SubmitCmd()
            submit.main(submit.parser.parse_args(["-s", jobspec]))
        elif args.mini == "cancel":
            cancel = CancelCmd()
            cancel.main(cancel.parser.parse_args(["-s", jobspec]))
        else:
            self.logger.error("Error: unrecognized mini command")


class FluxKubeDaemon:
    """
    FluxKubeDaemon registers the fluxkube service,
    and starts a request message watcher callback
    which submits or cancels jobspec payload from
    the flux-kube jobtap plugin.
    """

    def __init__(self):
        self.logger = logging.getLogger("flux-kube")
        self.flux_h = flux.Flux()

    def main(self, args):
        # Register the service in main since @util.CLIMain
        # will invoke __init__ for each CLI command
        # execution
        self.flux_h.service_register("fluxkube").get()
        submit = SubmitCmd()
        cancel = CancelCmd()
        self.submit_parser = submit.parser
        self.cancel_parser = cancel.parser

        def submit_jobspec(flux_h, t, msg, arg):
            try:
                submit.main(
                    self.submit_parser.parse_args(["-s", msg.payload["jobspec"]])
                )
            except Exception as e:
                self.logger.error(e)
                flux_h.respond_error(msg,  errno.EPROTO, None)
            else:
                flux_h.respond(msg)

        def cancel_jobspec(flux_h, t, msg, arg):
            try:
                cancel.main(
                    self.cancel_parser.parse_args(["-s", msg.payload["jobspec"]])
                )
            except Exception as e:
                self.logger.error(e)
                flux_h.respond_error(msg,  errno.EPROTO, None)
            else:
                flux_h.respond(msg)

        submit_watcher = self.flux_h.msg_watcher_create(
            submit_jobspec, FLUX_MSGTYPE_REQUEST, "fluxkube.submit"
        )
        cancel_watcher = self.flux_h.msg_watcher_create(
            cancel_jobspec, FLUX_MSGTYPE_REQUEST, "fluxkube.cancel"
        )
        submit_watcher.start()
        cancel_watcher.start()

        self.flux_h.reactor_run()


LOGGER = logging.getLogger("flux-kube")


@util.CLIMain(LOGGER)
def main():

    urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)

    parser = argparse.ArgumentParser(prog="flux-kube")
    subparsers = parser.add_subparsers(
        title="supported subcommands", description="", dest="subcommand"
    )
    subparsers.required = True

    get = GetCmd()
    get_parser_sub = subparsers.add_parser(
        "get",
        parents=[get.get_parser()],
        help="get template names, parameters, and values, "
        "and common OpenShift/K8s objects",
        formatter_class=argparse.RawTextHelpFormatter,
    )
    get_parser_sub.set_defaults(func=get.main)

    translate = TranslateCmd()
    translate_parser_sub = subparsers.add_parser(
        "translate",
        parents=[translate.get_parser()],
        help="translate jobspec into K8s template",
        formatter_class=util.help_formatter(),
    )
    translate_parser_sub.set_defaults(func=translate.main)

    submit = SubmitCmd()
    submit_parser_sub = subparsers.add_parser(
        "submit",
        parents=[submit.get_parser()],
        help="submit jobspec into K8s template",
        formatter_class=util.help_formatter(),
    )
    submit_parser_sub.set_defaults(func=submit.main)

    cancel = CancelCmd()
    cancel_parser_sub = subparsers.add_parser(
        "cancel",
        parents=[cancel.get_parser()],
        help="delete objects corresponding to Openshift template",
        formatter_class=util.help_formatter(),
    )
    cancel_parser_sub.set_defaults(func=cancel.main)

    mini = MiniCmd()
    mini_parser_sub = subparsers.add_parser(
        "mini",
        parents=[mini.get_parser()],
        help="generates a skeleton jobspec with the specified "
        "OpenShift template and template parameter overrides and "
        "creates the corresponding objects (submit) "
        "or deletes them (cancel)",
        formatter_class=util.help_formatter(),
    )
    mini_parser_sub.set_defaults(func=mini.main)

    daemon = FluxKubeDaemon()
    daemon_parser_sub = subparsers.add_parser(
        "daemonize",
        help="daemonize the submission",
        formatter_class=util.help_formatter(),
    )
    daemon_parser_sub.set_defaults(func=daemon.main)

    args = parser.parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
