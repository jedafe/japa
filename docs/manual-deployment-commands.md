 How do I trigger the deployment manually?

 Two ways:

 GitHub UI

   1. Go to the repo → Actions tab → Deploy Staging (left sidebar)
   2. Click Run workflow (top right)
   3. Leave  apps  =  changed  (or type  all  /  api,docs ) → Run workflow

 CLI

 
   # deploy only what changed since last deploy
   gh workflow run deploy.yml
 
   # everything
   gh workflow run deploy.yml -f apps=all
 
   # specific apps
   gh workflow run deploy.yml -f apps=api,main
 
   # custom diff base
   gh workflow run deploy.yml -f apps=changed -f base=main~5
   
   
   # Manua CI run
   gh workflow run ci.yml
 

 Then watch:  gh run watch  or Actions tab —  filter  decides the matrix,  build-images  builds + pushes to  registry.japa.to ,  deploy-staging  fires the Coolify
 webhooks.
 
  gh workflow run ci.yml && sleep 10 \
     && gh run watch "$(gh run list --workflow=ci.yml --event workflow_dispatch -L 1 --json databaseId -q '.[0].databaseId')" \
          --exit-status \
     && gh workflow run deploy.yml

