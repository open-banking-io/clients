// Open Banking Settings — client-side test-connection button

frappe.ui.form.on("Open Banking Settings", {
    refresh(frm) {
        // test_connection is restricted server-side to these roles; don't
        // offer the button to users who would just get a PermissionError.
        if (!frappe.user.has_role("Accounts Manager") && !frappe.user.has_role("System Manager")) {
            return;
        }
        if (frm.custom_buttons && frm.custom_buttons[__("Test Connection")]) {
            return;
        }
        frm.add_custom_button(__("Test Connection"), function () {
            frappe.call({
                method:
                    "erpnext_open_banking.erpnext_open_banking.doctype.open_banking_settings.open_banking_settings.test_connection",
                freeze: true,
                freeze_message: __("Testing connection..."),
                callback: function (r) {
                    if (r.message) {
                        // The message can embed server/exception text — escape it.
                        const message = frappe.utils.escape_html(String(r.message.message ?? ""));
                        frappe.msgprint({
                            title: r.message.success ? __("Success") : __("Error"),
                            message,
                            indicator: r.message.success ? "green" : "red",
                        });
                    }
                },
            });
        });
    },
});
