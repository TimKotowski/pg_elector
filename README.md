# pg_elector is a simple leader election. 

Leadership mechanisms are great when working in high availability environments, where certain processes require idempotency, 
consistency, and readability. To ensure isolated work on one node. If you are already using postgres, this is a simple way
to ensure these type of guarantees, without needing to use other external tools.