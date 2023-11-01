# InVitro

In-Vitro is a set of tools for analyzing the performance of serverless cluster deployments. In-Vitro consists of two tools, namely sampler and loader. Sampler creates representative workload summaries (i.e., samples of functions) based on production traces. Loader reconstructs the invocation traffic based on a given trace and steers this load to the functions deployed in the studied serverless cluster. Currently, In-Vitro supports [vHive](https://github.com/vhive-serverless/vHive) and [OpenWhisk](https://openwhisk.apache.org/). Documentation on how to use the sampler and the loader can be found in the `docs` folder.

Standard sampled traces are available in [data/traces/reference](data/traces/reference/) folder in this repository. The traces are sampled from the Azure Functions production traces using the [sampler](sampler) tool. More details on the sampling process can be found [here](docs/sampler.md#reference-traces).

## Reference our work

```
@inproceedings{ustiugov:invitro,
  author    = {Dmitrii Ustiugov and
               Dohyun Park and
               Lazar Cvetković and
               Mihajlo Djokic and
               Hongyu He and
               Boris Grot and
               Ana Klimovic},
  title     = {Enabling In-Vitro Serverless Systems Research},
  booktitle = {Proceedings of the 4th Workshop on Resource Disaggregation and Serverless (WORDS 2023)},
  publisher = {{ACM}},
  year      = {2023},
}
```

## Developing InVitro

### Getting help and contributing

We would be happy to answer any questions in GitHub Issues and encourage the open-source community
to submit new Issues, assist in addressing existing issues and limitations, and contribute their code with Pull Requests.
Please check our guide on [Contributing to vHive](https://github.com/vhive-serverless/vHive/docs/contributing_to_vhive.md) if you would like contribute.
You can also talk to us in our [Slack space](https://join.slack.com/t/vhivetutorials/shared_invite/zt-1fk4v71gn-nV5oev5sc9F4fePg3_OZMQ).


## License and copyright

InVitro is free. We publish the code under the terms of the MIT License that allows distribution, modification, and commercial use.
This software, however, comes without any warranty or liability.

The software is maintained by the [EASL lab](https://systems.ethz.ch/research/easl.html) at ETH Zürich.

## Maintainers

* [Lazar Cvetkovic](https://github.com/cvetkovic) - lazar.cvetkovic@inf.ethz.ch