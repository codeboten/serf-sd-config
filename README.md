# Serf Service Discovery for Prometheus

Adapter to support service discovery based on members of a serf cluster. The code in this repo is based on the example Prometheus [adapter](https://github.com/prometheus/prometheus/blob/master/documentation/examples/custom-sd/adapter-usage/main.go).

Some assumptions:

* at this point, the adapter assumes the members of the serf cluster expose prometheus metrics on port 9100