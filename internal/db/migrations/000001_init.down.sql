DROP TRIGGER IF EXISTS book_metadata_search_trg ON book_metadata;
DROP FUNCTION IF EXISTS book_metadata_search_update();
DROP TABLE IF EXISTS app_settings, opds_users, bookdrop_files, user_book_progress,
  magic_shelves, shelf_books, shelves, book_categories, categories, book_authors, authors,
  book_metadata, books, library_paths, libraries, sessions, user_settings, user_permissions, users CASCADE;
