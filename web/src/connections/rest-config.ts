import type { RESTConfig } from '../api'
export const emptyREST = (): RESTConfig => ({auth:{type:'none'},forwardIdentity:false,requireIdentity:false,allowInsecureHttp:false,timeoutSeconds:30,operations:[]})
