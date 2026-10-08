DROP SEQUENCE IF EXISTS request_number_seq, quote_number_seq, order_number_seq, support_number_seq, appointment_number_seq;
DROP TABLE IF EXISTS wishlist_items, notifications, uploads, addresses, customers, content_blocks, feature_flags,
  business_settings, status_history, audit_logs, password_resets, sessions, users, roles CASCADE;
DROP FUNCTION IF EXISTS audit_logs_immutable();
DROP FUNCTION IF EXISTS set_updated_at();
