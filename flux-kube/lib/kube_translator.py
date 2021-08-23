##############################################################
# Copyright 2021 Lawrence Livermore National Security, LLC
# (c.f. AUTHORS, NOTICE.LLNS, COPYING)
#
# This file is part of the Flux resource manager framework.
# For details, see https://github.com/flux-framework.
#
# SPDX-License-Identifier: LGPL-3.0
##############################################################

import warnings


class JobspecTranslator:
    """
    JobspecTranslator is a base class for mapping and translating
    a jobspec with template overrides into a Kubernetes
    or OpenShift template.
    """

    def __init__(self, jobspec_dict, templates_dict):
        """
        args:
             jobspec_dict is the YAML jobspec ingested as a dict
             templates_dict is a dictionary keyed by the cluster
                  service template names, with values of the templates
                  themselves
        """
        self.jobspec = jobspec_dict
        self.templates = templates_dict

        # Check for multiple services per jobspec, which is
        # currently unsupported.
        if len(self.jobspec["tasks"]) > 1:
            raise NotImplementedError(
                "Requesting multiple services per " "jobspec is currently unsupported\n"
            )

        if len(self.jobspec["tasks"][0]["command"]) > 1:
            raise NotImplementedError(
                "Requesting multiple services per " "jobspec is currently unsupported\n"
            )

        # Check if the service name requested matches a known template
        try:
            self.service = {
                "template": self.templates[(self.jobspec["tasks"][0]["command"][0])]
            }
        except KeyError:
            print(
                "service %s not a registered template on the cluster\n"
                % self.jobspec["tasks"][0]["command"]
            )
            raise

    def translate(self):
        if "template_overrides" in self.jobspec["attributes"]["system"]:
            param_names = set()
            self.service["overrides"] = {}
            for override in self.jobspec["attributes"]["system"]["template_overrides"]:
                matched = False
                for param in self.service["template"]["parameters"]:
                    if override in param_names:
                        (self.service["overrides"][override]) = self.jobspec[
                            "attributes"
                        ]["system"]["template_overrides"][override]
                        matched = True
                    else:
                        if param["name"] not in param_names:
                            param_names.update([param["name"]])
                        if param["name"] == override:
                            (self.service["overrides"][override]) = self.jobspec[
                                "attributes"
                            ]["system"]["template_overrides"][override]
                            matched = True
                if not matched:
                    warnings.warn(
                        "Warning: specified overrides do not match "
                        "template parameters\n",
                        SyntaxWarning,
                    )

            return self.service
