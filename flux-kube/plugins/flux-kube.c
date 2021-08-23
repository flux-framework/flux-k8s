/************************************************************\
 * Copyright 2021 Lawrence Livermore National Security, LLC
 * (c.f. AUTHORS, NOTICE.LLNS, COPYING)
 *
 * This file is part of the Flux resource manager framework.
 * For details, see https://github.com/flux-framework.
 *
 * SPDX-License-Identifier: LGPL-3.0
\************************************************************/

/* flux-kube.c - If attributes.system.R exists in jobspec, then
 *  bypass scheduler alloc protocol and use R directly (for instance
 *  owner use only)
 */

#include <unistd.h>
#include <sys/types.h>

#include <jansson.h>
#include <flux/core.h>
#include <flux/jobtap.h>

static void alloc_continuation (flux_future_t *f, void *arg)
{
    flux_plugin_t *p = arg;
    flux_jobid_t *idptr = flux_future_aux_get (f, "jobid");

    if (flux_future_get (f, NULL) < 0) {
        flux_jobtap_raise_exception (p,
                                     *idptr,
                                     "alloc", 0,
                                     "failed to commit R to kvs: %s",
                                      strerror (errno));
        goto done;
    }
    if (flux_jobtap_event_post_pack (p,
                                     *idptr,
                                     "alloc",
                                     "{s:b}",
                                     "bypass", true) < 0)
        flux_jobtap_raise_exception (p,
                                     *idptr,
                                     "alloc", 0,
                                     "failed to post alloc event: %s",
                                     strerror (errno));

    /*  Set "needs-free" so that flux-kube knows that a "free"
     *  event needs to be emitted for this node.
     */
    if (flux_jobtap_job_aux_set (p, *idptr, "flux-kube::free", p, NULL) < 0)
        flux_log_error (flux_jobtap_get_flux (p),
                        "id=%ju: Failed to set flux-kube::free",
                        *idptr);

done:
    flux_future_destroy (f);
}

static int alloc_start (flux_plugin_t *p,
                        flux_jobid_t id,
                        const char *R)
{
    flux_t *h;
    char key[64];
    flux_future_t *f = NULL;
    flux_kvs_txn_t *txn = NULL;
    flux_jobid_t *idptr = NULL;

    if (!(h = flux_jobtap_get_flux (p))
        || flux_job_kvs_key (key, sizeof (key), id, "R") < 0
        || !(txn = flux_kvs_txn_create ())
        || flux_kvs_txn_put (txn, 0, key, R) < 0
        || !(f = flux_kvs_commit (h, NULL, 0, txn))
        || flux_future_then (f, -1, alloc_continuation, p)
        || !(idptr = calloc (1, sizeof (*idptr)))
        || flux_future_aux_set (f, "jobid", idptr, free) < 0) {
        flux_kvs_txn_destroy (txn);
        flux_future_destroy (f);
        free (idptr);
        return -1;
    }
    *idptr = id;
    flux_kvs_txn_destroy (txn);
    return 0;
}


static int sched_cb (flux_plugin_t *p,
                     const char *topic,
                     flux_plugin_arg_t *args,
                     void *arg)
{
    const char *R, *jobspec = NULL;
    flux_jobid_t id;
    flux_future_t *f;
    int retval = -1;

    /*  If flux-kube::R set on this job then commit R to KVS,
     *  get the jobspec, submit the jobspec to flux-kube.py, 
     *  and set flux-kube flag.
     */
    if (!(R = flux_jobtap_job_aux_get (p,
                                       FLUX_JOBTAP_CURRENT_JOB,
                                       "flux-kube::R")))
        return 0;

    if (!(jobspec = flux_jobtap_job_aux_get (p,
                                       FLUX_JOBTAP_CURRENT_JOB,
                                       "flux-kube::jobspec")))
        return 0;

    if (flux_plugin_arg_unpack (args,
                                FLUX_PLUGIN_ARG_IN,
                                "{s:I}",
                                "id", &id) < 0) {
        flux_jobtap_raise_exception (p, FLUX_JOBTAP_CURRENT_JOB,
                                     "alloc", 0,
                                     "flux-kube: %s: unpack: %s",
                                     topic,
                                     flux_plugin_arg_strerror (args));
        return -1;
    }

    if (!(f = flux_rpc_pack (flux_jobtap_get_flux (p), "fluxkube.submit", 
                             FLUX_NODEID_ANY, 0,
                             "{s:s}", "jobspec", jobspec))) { 
        flux_jobtap_raise_exception (p, FLUX_JOBTAP_CURRENT_JOB,
                                     "alloc", 0,
                                     "flux-kube: %s: Failed to send RPC: %s",
                                     topic,
                                     strerror (errno));
        return -1;
    }
    
    if (flux_rpc_get_unpack (f, "{s:i}", "retval", &retval) != 0) {
        flux_jobtap_raise_exception (p, FLUX_JOBTAP_CURRENT_JOB,
                                     "alloc", 0,
                                     "flux-kube: %s: Failed to "
                                     "unpack RPC: %s",
                                     topic,
                                     strerror (errno));
        return -1;
    }

    if (retval != 0) {
        flux_jobtap_raise_exception (p, FLUX_JOBTAP_CURRENT_JOB,
                                     "alloc", 0,
                                     "flux-kube.py: %s: submit callback "
                                     "failed: %s",
                                     topic,
                                     strerror (errno));
        return -1;
    }

    if (alloc_start (p, id, R) < 0)
        flux_jobtap_raise_exception (p, id, "alloc", 0,
                                     "failed to commit R to kvs");

    if (flux_jobtap_job_set_flag (p,
                                  FLUX_JOBTAP_CURRENT_JOB,
                                  "alloc-bypass") < 0)
        return flux_jobtap_raise_exception (p, FLUX_JOBTAP_CURRENT_JOB,
                                            "alloc", 0,
                                            "Failed to set alloc-bypass: %s",
                                            strerror (errno));
    return 0;
}

static int cleanup_cb (flux_plugin_t *p,
                       const char *topic,
                       flux_plugin_arg_t *args,
                       void *arg)
{
    /*  If flux-kube::free is set on this job, then this plugin
     *   sent an "alloc" event, so a "free" event needs to be sent now.
     */
    if (flux_jobtap_job_aux_get (p,
                                 FLUX_JOBTAP_CURRENT_JOB,
                                 "flux-kube::free")) {
        if (flux_jobtap_event_post_pack (p,
                                         FLUX_JOBTAP_CURRENT_JOB,
                                         "free",
                                         NULL) < 0)
             flux_log_error (flux_jobtap_get_flux (p),
                             "flux-kube: failed to post free event");
    }
    return 0;
}

static int validate_cb (flux_plugin_t *p,
                        const char *topic,
                        flux_plugin_arg_t *args,
                        void *arg)
{
    int fluxkube = 0;
    json_t *jobspec = NULL;
    char *js, *s;
    uint32_t userid = (uint32_t) -1;

    if (flux_plugin_arg_unpack (args,
                                FLUX_PLUGIN_ARG_IN,
                                "{s:i s:{s:{s:{s?i}}}}",
                                "userid", &userid,
                                "jobspec",
                                 "attributes",
                                  "system",
                                   "flux-kube", &fluxkube) < 0) {
        return flux_jobtap_reject_job (p,
                                       args,
                                       "invalid system.flux-kube: %s",
                                       flux_plugin_arg_strerror (args));
    }

    /*  Nothing to do if not fluxkube
     */
    if (!fluxkube)
        return 0;

    if (userid != getuid ())
        return flux_jobtap_reject_job (p,
                                       args,
                                       "Guest user cannot use alloc bypass");

    if (flux_plugin_arg_unpack (args,
                                FLUX_PLUGIN_ARG_IN,
                                "{s:o}",
                                "jobspec", &jobspec) < 0) {
        return flux_jobtap_reject_job (p,
                                       args,
                                       "invalid jobspec: %s",
                                       flux_plugin_arg_strerror (args));
    }
   /*  Store jobspec in job structure to avoid re-fetching from plugin args
     *   in job.state.sched callback.
     */
    if (!(js = json_dumps (jobspec, 0))
        || flux_jobtap_job_aux_set (p,
                                    FLUX_JOBTAP_CURRENT_JOB,
                                    "flux-kube::jobspec",
                                    js,
                                    free) < 0) {
        free (js);
        return flux_jobtap_reject_job (p,
                                       args,
                                       "failed to capture flux-kube "
                                       "jobspec: %s",
                                       strerror (errno));
    }
    /*  Store R string in job structure to avoid re-fetching from plugin args
     *   in job.state.sched callback.
     */
    s = "{\"version\": 1, "
          "\"execution\": "
            "{\"R_lite\": "
              "[{\"rank\": \"0\", "
                "\"children\": "
                  "{\"core\": \"0\"}}], "
              "\"starttime\": 0.0, "
              "\"expiration\": 0.0, "
              "\"nodelist\": "
              "[\"kubernetes\"]}}";
    if (flux_jobtap_job_aux_set (p,
                                 FLUX_JOBTAP_CURRENT_JOB,
                                 "flux-kube::R", 
                                 strdup (s), 
                                 free) < 0)
        return flux_jobtap_reject_job (p,
                                       args,
                                       "failed to capture flux-kube R: %s",
                                       strerror (errno));
    return 0;
}

static const struct flux_plugin_handler tab[] = {
    { "job.state.sched",   sched_cb,    NULL },
    { "job.state.cleanup", cleanup_cb,  NULL },
    { "job.validate",      validate_cb, NULL },
    { 0 }
};

int flux_plugin_init (flux_plugin_t *p)
{
    return flux_plugin_register (p, "flux-kube", tab);
}
