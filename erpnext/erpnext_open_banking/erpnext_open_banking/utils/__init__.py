# ERPNext Open Banking — utils package
#
# Imports are lazy: submodules (envelope, client, mapper) can be imported
# directly without needing frappe. Only the connector requires frappe.

__all__ = [
    "OpenBankingClient",
    "sync_all_connections",
    "sync_connection",
    "sync_now",
    "map_transaction",
]


def __getattr__(name):
    if name == "OpenBankingClient":
        from .client import OpenBankingClient
        return OpenBankingClient
    if name == "map_transaction":
        from .mapper import map_transaction
        return map_transaction
    if name in ("sync_all_connections", "sync_connection", "sync_now"):
        from . import connector
        return getattr(connector, name)
    raise AttributeError(f"module {__name__!r} has no attribute {name!r}")
