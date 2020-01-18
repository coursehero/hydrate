/*
Package hydrate provides functionality to expand preloading functionality of gorm.

Specifically gorm hydrate will only support loading each relationship as a different query. This results in a query per
level. Additionally each level loads the data by using a WHERE IN (...primary keys) which in certain situations with
deep hierarchies with thousands of items can result in poor performing queries.

There are two ways provided to load data into a hierarchy using raw queries. Query will perform a
single query to load one or more model structs. MultiQuery has a slice of Queries as its backing type and will allow
multiple queries to be run and have results combined.
*/
package hydrate
