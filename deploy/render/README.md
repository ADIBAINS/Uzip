# Uzip on Render

The Blueprint runs a public portfolio demo on one free web service with free Render Postgres and Render Key Value. The dashboard uses a shared demo workspace. Short links point to example.com, example.org, or example.net, and analytics are stored in PostgreSQL. Country and referrer breakdowns display Unknown in this demo. The full Kafka and ClickHouse pipeline runs via the repository's Docker Compose setup.

1. Push the branch containing `render.yaml` to GitHub.
2. In the [Render Dashboard](https://dashboard.render.com/), select **New → Blueprint**, connect the Uzip repository, and choose that branch. Apply the Blueprint with all three resources on the free plan.
3. Wait for the web service to show **Live** and open its public URL. Enter the demo workspace, create a short link to `https://example.com/`, open it in a browser, and refresh Analytics to confirm the click appears.

The free database expires after 30 days, and the free web service sleeps after idle periods. [Render free-tier limits](https://render.com/docs/free)
