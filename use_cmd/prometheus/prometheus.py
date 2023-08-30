import requests


class PrometheusClient:
    def __init__(self, host: str, port: int):
        if not host.startswith("http"):
            host = f"http://{host}"
        self.__query_url = f"{host}:{port}/api/v1/"

    def get_query_url(self, query_type: str):
        return self.__query_url + query_type

    def get_instant(self, query: str, time: float = None):
        """
        :param query: PromQL query in string format
        :param time: unix timestamp seconds
        :return: return a tuple of (unix_time, query_value)
        """

        params = {"query": query}
        if time:
            params.update(time=time)
        response = requests.get(self.get_query_url("query"), params=params).json()
        if response["status"] != "success":
            raise Exception("Unsuccessful instant query")
        try:
            return response["data"]["result"]["value"]
        except TypeError:
            return list(map(lambda r: r["value"], response["data"]["result"]))

    def get_range(self, query: str, start_time: float, end_time: float, step: int):
        """
        :param query: PromQL query in string format
        :param start_time: unix timestamp seconds
        :param end_time: unix timestamp seconds
        :return: return a list of tuples. i.e. [(unix_time, query_value), (unix_time, query_value), ...]
        """
        response = requests.get(
            self.get_query_url("query_range"),
            params={"query": query, "start": start_time, "end": end_time, "step": f"{step}s"}
        ).json()
        print(response)
        if response["status"] != "success":
            raise Exception("Unsuccessful range query")
        try:
            return response["data"]["result"].get("values")
        except AttributeError:
            try:
                return response["data"]["result"][0].get("values")
            except (KeyError, IndexError, AttributeError):
                return response["data"]["result"]