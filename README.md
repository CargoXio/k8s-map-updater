Update map when pods change
===========================

This is a really simple container that changes the contents of the map if the pods
are created, updated or deleted.

Why
---

This was done merely for the need of Redis HAProxy:
- the proxy needs a configuration file (haproxy.cfg),
- the configuration file needs to include a list of all the pods where redis is running
- pods do not have a DNS name and can only be accessed through a `Service` (which connects
  to any of the available pods) or directly by IP address,
- IP addresses change if the pods are restarted, added or removed.

Hence the configuration might become invalid if the pods are restarted and would need
to be recreated.

How
---

1. This simple tool watches for changes to the `Pods` in its namespace. Whenever a pod 
   is  added, removed or updated, a trigger is fired.
1. As a convinience, the trigger will fire if the template file is changed, as well.
1. The trigger will take the supplied template, pass in the `{{ .pods }}` configuration
   and create a new configuration.
1. It will update the configured `ConfigMap` with new configuration file.
1. It is up to the user (e.g. through pod annotation with [reloader](https://github.com/stakater/Reloader))
   to restart ha-proxy when this configuration is changed.
   
Further work
------------

This software is really simple, and it does not filter out anything -- e.g. it will catch
any change in any pod -- whether relevant or not. Be wary when using it in namespaces with
many pods.
