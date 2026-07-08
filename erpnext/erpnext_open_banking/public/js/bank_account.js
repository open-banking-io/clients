// Bank Account form — adds "Sync Now" button that triggers open-banking.io sync

frappe.ui.form.on("Bank Account", {
    refresh(frm) {
        // Open Banking Connection is restricted to these roles — skip the
        // lookup (and the sync button) for everyone else.
        if (!frappe.user.has_role("Accounts Manager") && !frappe.user.has_role("System Manager")) {
            return;
        }
        // Only show button if an Open Banking Connection is linked to this Bank Account
        frappe.db
            .get_value("Open Banking Connection", { bank_account: frm.doc.name }, "name")
            .then((r) => {
                if (r && r.message && r.message.name) {
                    frm.add_custom_button(
                        __("Sync from Open Banking"),
                        function () {
                            frappe.call({
                                method:
                                    "erpnext_open_banking.erpnext_open_banking.utils.connector.sync_now",
                                args: { bank_account: frm.doc.name },
                                freeze: true,
                                freeze_message: __("Syncing transactions from open-banking.io..."),
                                callback: function (response) {
                                    if (response.message) {
                                        const result = response.message;
                                        let msg = __(
                                            "Sync complete: {0} new, {1} skipped (dedup).",
                                            [result.created, result.skipped]
                                        );
                                        if (result.errors && result.errors.length > 0) {
                                            msg +=
                                                "<br><br><strong>Warnings:</strong><br>" +
                                                result.errors.join("<br>");
                                        }
                                        frappe.msgprint({
                                            title: __("Open Banking Sync"),
                                            message: msg,
                                            indicator: result.errors.length > 0 ? "orange" : "green",
                                        });
                                        frm.refresh();
                                    }
                                },
                            });
                        },
                        __("Open Banking")
                    );
                }
            })
            .catch(() => {
                // No access to Open Banking Connection — just skip the button.
            });
    },
});
